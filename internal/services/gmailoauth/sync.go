package gmailoauth

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mandloideep/miniclaw/internal/db/sqlcgen"
	"github.com/mandloideep/miniclaw/internal/services/account"
)

// AttachmentStore is the optional sink for inline images we extract from
// Gmail multipart bodies. Implemented by *attachments.Service. nil-safe
// inside the syncer for tests that don't care about attachments.
type AttachmentStore interface {
	Store(ctx context.Context, emailID int64, contentID, filename, mime string, isInline bool, data []byte) error
}

// Triage is the optional block-rule interface used by ingest to drop
// senders the user has marked as blocked. Mirrors the IMAP syncer's
// triage hook so both paths apply the same filter rules.
type Triage interface {
	MatchesBlock(ctx context.Context, accountID int64, fromAddress string) (bool, error)
	RegisterSender(ctx context.Context, accountID int64, address string) error
}

// threadDeriver computes a stable thread anchor from RFC822 headers.
// Wired to email.DeriveThreadID at runtime via AttachThreadDeriver to
// avoid an import cycle between gmailoauth and email (email already
// imports nothing from us today; keep it that way).
type threadDeriver func(messageID, inReplyTo, references string) string

// Syncer ingests one Gmail-OAuth account at a time using the REST API.
type Syncer struct {
	q            *sqlcgen.Queries
	accounts     *account.Service
	attachments  AttachmentStore
	triage       Triage
	deriveThread threadDeriver
}

// NewSyncer wires the syncer to the shared pool and account service.
func NewSyncer(pool *sql.DB, acc *account.Service) *Syncer {
	return &Syncer{q: sqlcgen.New(pool), accounts: acc}
}

// AttachInlineStore wires an attachments sink in place. Returning a
// concrete *Syncer from a method would make the Wails binding generator
// expose Syncer as both a service and a model, which collides — so this
// mutates and returns nothing.
func (s *Syncer) AttachInlineStore(store AttachmentStore) {
	s.attachments = store
}

// AttachTriage wires a triage service in place so blocked senders are
// dropped before upsert and first-seen senders register in the screener.
func (s *Syncer) AttachTriage(t Triage) {
	s.triage = t
}

// AttachThreadDeriver wires a thread-id derivation function so the
// Gmail syncer produces the same anchor the IMAP syncer does. Empty
// derivation just stores the message-id; explicit so tests can
// inject a deterministic fake.
func (s *Syncer) AttachThreadDeriver(fn func(messageID, inReplyTo, references string) string) {
	s.deriveThread = fn
}

// Sync pulls messages from the user's INBOX newer than the account's
// last_synced_at (or fetch_since if that's later). Returns count written.
//
// On accounts that already have a gmail_history_id watermark, we use
// users.history.list for cheap incremental sync; otherwise the first
// pass falls back to messages.list with a date-bounded query. Dedupe is
// on (account_id, message_id) via the schema's UNIQUE constraint.
func (s *Syncer) Sync(ctx context.Context, accountID int64) (int, error) {
	acc, err := s.accounts.Get(ctx, accountID)
	if err != nil {
		return 0, err
	}
	if acc.AuthKind != account.AuthGmailOAuth {
		return 0, fmt.Errorf("account %d is not Gmail OAuth", accountID)
	}
	src, err := TokenSource(ctx, secretRefFor(acc.EmailAddress))
	if err != nil {
		return 0, err
	}
	client := oauth2HTTPClient(ctx, src)

	historyID, _ := s.q.GetGmailHistoryID(ctx, accountID)
	var (
		ids        []string
		nextCursor string
	)
	if historyID != "" {
		ids, nextCursor, err = listHistoryMessageIDs(ctx, client, historyID)
		if err != nil {
			// history cursors expire after ~7 days. Fall back to the
			// date-bounded query and re-seed the cursor at end-of-pass.
			ids = nil
			nextCursor = ""
		}
	}
	if len(ids) == 0 && nextCursor == "" {
		q := gmailWatermarkQuery(acc.FetchSince, acc.LastSyncedAt)
		ids, err = listMessageIDs(ctx, client, q)
		if err != nil {
			return 0, err
		}
	}
	written := s.ingestMessages(ctx, client, accountID, ids)
	// Always re-seed the history cursor after a pass: either the one
	// users.history.list handed back, or the freshest profile head if
	// we fell back to the date query.
	if nextCursor == "" {
		nextCursor, _ = fetchProfileHistoryID(ctx, client)
	}
	if nextCursor != "" {
		_ = s.q.UpdateGmailHistoryID(ctx, sqlcgen.UpdateGmailHistoryIDParams{
			GmailHistoryID: nextCursor, ID: accountID,
		})
	}
	if err := s.accounts.MarkSynced(ctx, accountID); err != nil {
		return written, err
	}
	return written, nil
}

// ingestMessages fetches each id, applies block rules, and upserts.
// Returns the count of new/updated rows. Shared by Sync and BackfillBefore.
func (s *Syncer) ingestMessages(ctx context.Context, client *http.Client, accountID int64, ids []string) int {
	written := 0
	for _, id := range ids {
		if cerr := ctx.Err(); cerr != nil {
			return written
		}
		msg, ferr := fetchMessage(ctx, client, id)
		if ferr != nil {
			continue
		}
		if s.triage != nil {
			blocked, blockErr := s.triage.MatchesBlock(ctx, accountID, msg.FromAddress)
			if blockErr == nil && blocked {
				continue
			}
			_ = s.triage.RegisterSender(ctx, accountID, msg.FromAddress)
		}
		threadID := msg.MessageID
		if s.deriveThread != nil {
			threadID = s.deriveThread(msg.MessageID, msg.InReplyTo, msg.References)
		}
		id2, err := s.q.UpsertEmail(ctx, sqlcgen.UpsertEmailParams{
			AccountID:   accountID,
			MessageID:   msg.MessageID,
			Folder:      "INBOX",
			Uid:         sql.NullInt64{}, // Gmail has no IMAP UID via REST
			FromAddress: msg.FromAddress,
			FromName:    msg.FromName,
			ToAddresses: msg.To,
			CcAddresses: msg.Cc,
			Subject:     msg.Subject,
			ReceivedAt:  msg.ReceivedAt,
			BodyPlain:   msg.BodyPlain,
			BodyHtml:    msg.BodyHTML,
			HeadersJson: msg.HeadersJSON,
			InReplyTo:   msg.InReplyTo,
			EmailRefs:   msg.References,
			ThreadID:    threadID,
		})
		if err != nil {
			continue
		}
		if msg.Category != "" {
			_ = s.q.SetCategory(ctx, sqlcgen.SetCategoryParams{
				Category: msg.Category, ID: id2,
			})
		}
		s.persistLabels(ctx, id2, msg.Labels)
		s.persistAttachments(ctx, client, msg.GmailID, id2, msg.Attachments)
		written++
	}
	return written
}

// persistLabels rewrites the label set for an email — full replacement
// is fine because Gmail returns the entire labelIds list on each fetch.
func (s *Syncer) persistLabels(ctx context.Context, emailID int64, labels []string) {
	if ctx.Err() != nil {
		return
	}
	_ = s.q.ClearEmailLabels(ctx, emailID)
	for _, label := range labels {
		if label == "" {
			continue
		}
		_ = s.q.AddEmailLabel(ctx, sqlcgen.AddEmailLabelParams{
			EmailID: emailID, Label: label,
		})
	}
}

// gmailWatermarkQuery builds a Gmail search query that bounds the result
// set to messages newer than last_synced_at or fetch_since.
func gmailWatermarkQuery(fetchSince, lastSynced string) string {
	var newest string
	for _, v := range []string{fetchSince, lastSynced} {
		if v == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			s := t.Format("2006/01/02")
			if s > newest {
				newest = s
			}
		} else if t, err := time.Parse("2006-01-02", v); err == nil {
			s := t.Format("2006/01/02")
			if s > newest {
				newest = s
			}
		}
	}
	if newest == "" {
		return "in:inbox"
	}
	return fmt.Sprintf("in:inbox after:%s", newest)
}

// listMessageIDs hits messages.list with the watermark query.
func listMessageIDs(ctx context.Context, client *http.Client, q string) ([]string, error) {
	return listMessageIDsPaged(ctx, client, q, 200, 1)
}

// listMessageIDsPaged walks messages.list with pagination. Returns up to
// pageSize * maxPages IDs. Stops on first empty page or absent nextPageToken.
func listMessageIDsPaged(ctx context.Context, client *http.Client, q string, pageSize, maxPages int) ([]string, error) {
	var (
		all       []string
		pageToken string
	)
	for range maxPages {
		u := fmt.Sprintf("https://gmail.googleapis.com/gmail/v1/users/me/messages?q=%s&maxResults=%d",
			url.QueryEscape(q), pageSize)
		if pageToken != "" {
			u += "&pageToken=" + url.QueryEscape(pageToken)
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := client.Do(req)
		if err != nil {
			return all, fmt.Errorf("messages.list: %w", err)
		}
		var body struct {
			Messages []struct {
				ID string `json:"id"`
			} `json:"messages"`
			NextPageToken string `json:"nextPageToken"`
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return all, fmt.Errorf("messages.list: %s", resp.Status)
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			_ = resp.Body.Close()
			return all, err
		}
		_ = resp.Body.Close()
		for _, m := range body.Messages {
			all = append(all, m.ID)
		}
		if body.NextPageToken == "" || len(body.Messages) == 0 {
			break
		}
		pageToken = body.NextPageToken
	}
	return all, nil
}

// BackfillBefore pulls historical messages strictly older than `before`
// (RFC3339 or YYYY-MM-DD). Used by the UI's "fetch older mail" action so
// the user can pull a window the watermark sync would never visit.
// Caps at `maxMessages` (≤ 1000) per call to bound API spend.
func (s *Syncer) BackfillBefore(ctx context.Context, accountID int64, before string, maxMessages int) (int, error) {
	if maxMessages <= 0 || maxMessages > 1000 {
		maxMessages = 200
	}
	acc, err := s.accounts.Get(ctx, accountID)
	if err != nil {
		return 0, err
	}
	if acc.AuthKind != account.AuthGmailOAuth {
		return 0, fmt.Errorf("account %d is not Gmail OAuth", accountID)
	}
	beforeDate, err := parseFlexDate(before)
	if err != nil {
		return 0, fmt.Errorf("before: %w", err)
	}
	src, err := TokenSource(ctx, secretRefFor(acc.EmailAddress))
	if err != nil {
		return 0, err
	}
	client := oauth2HTTPClient(ctx, src)

	q := fmt.Sprintf("in:inbox before:%s", beforeDate.Format("2006/01/02"))
	pageSize := 100
	pages := (maxMessages + pageSize - 1) / pageSize
	ids, err := listMessageIDsPaged(ctx, client, q, pageSize, pages)
	if err != nil {
		return 0, err
	}
	if len(ids) > maxMessages {
		ids = ids[:maxMessages]
	}
	return s.ingestMessages(ctx, client, accountID, ids), nil
}

// persistAttachments stores each inline/file attachment for the given
// email. Hydrates body data via attachments.get when the part was big
// enough that Gmail deferred it. nil-safe when no attachment store is
// wired in (e.g. unit tests).
func (s *Syncer) persistAttachments(ctx context.Context, client *http.Client, gmailMsgID string, emailID int64, atts []gmailAttachment) {
	if s.attachments == nil {
		return
	}
	for _, a := range atts {
		if ctx.Err() != nil {
			return
		}
		data := a.Data
		if len(data) == 0 && a.AttachmentID != "" {
			hydrated, err := fetchAttachmentData(ctx, client, gmailMsgID, a.AttachmentID)
			if err != nil || len(hydrated) == 0 {
				continue
			}
			data = hydrated
		}
		if len(data) == 0 {
			continue
		}
		_ = s.attachments.Store(ctx, emailID, a.ContentID, a.Filename, a.MIME, a.Inline, data)
	}
}

// SyncNow is a thin wrapper exposed to the frontend so the UI can trigger
// an immediate pull from a button without waiting for the scheduler tick.
// Returns the count of new/updated rows.
func (s *Syncer) SyncNow(ctx context.Context, accountID int64) (int, error) {
	return s.Sync(ctx, accountID)
}

// parseFlexDate accepts RFC3339 or YYYY-MM-DD and returns the UTC date.
func parseFlexDate(v string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("%q: expected RFC3339 or YYYY-MM-DD", v)
}

// gmailMessage is what fetchMessage returns once the REST payload is parsed.
type gmailMessage struct {
	MessageID   string
	GmailID     string // server-side id, used for attachments.get
	FromAddress string
	FromName    string
	To, Cc      string
	Subject     string
	ReceivedAt  string
	BodyPlain   string
	BodyHTML    string
	HeadersJSON string
	InReplyTo   string
	References  string
	Category    string   // mapped from Gmail labelIds; empty if not categorisable
	Labels      []string // raw labelIds; persisted to email_labels
	Attachments []gmailAttachment
}

func fetchMessage(ctx context.Context, client *http.Client, id string) (gmailMessage, error) {
	u := "https://gmail.googleapis.com/gmail/v1/users/me/messages/" + url.PathEscape(id) + "?format=full"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := client.Do(req)
	if err != nil {
		return gmailMessage{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return gmailMessage{}, fmt.Errorf("messages.get: %s", resp.Status)
	}
	var body struct {
		ID           string   `json:"id"`
		LabelIds     []string `json:"labelIds"`
		InternalDate string   `json:"internalDate"` // unix millis
		Payload      struct {
			Headers []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"headers"`
			Parts []part `json:"parts"`
			Body  struct {
				Data string `json:"data"`
			} `json:"body"`
			MimeType string `json:"mimeType"`
		} `json:"payload"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return gmailMessage{}, err
	}
	hdrs := map[string]string{}
	for _, h := range body.Payload.Headers {
		hdrs[h.Name] = h.Value
	}
	out := gmailMessage{
		MessageID:   hdrs["Message-ID"],
		GmailID:     body.ID,
		Subject:     hdrs["Subject"],
		ReceivedAt:  internalDateToRFC3339(body.InternalDate),
		BodyPlain:   walkParts(body.Payload.Parts, body.Payload.MimeType, body.Payload.Body.Data, "text/plain"),
		BodyHTML:    walkParts(body.Payload.Parts, body.Payload.MimeType, body.Payload.Body.Data, "text/html"),
		InReplyTo:   strings.TrimSpace(hdrs["In-Reply-To"]),
		References:  strings.TrimSpace(hdrs["References"]),
		Category:    categoryFromLabels(body.LabelIds),
		Labels:      body.LabelIds,
		Attachments: collectAttachments(body.Payload.Parts),
	}
	if out.MessageID == "" {
		out.MessageID = body.ID // fallback so dedupe still works
	}
	if h := hdrs["From"]; h != "" {
		out.FromName, out.FromAddress = splitAddress(h)
	}
	out.To = hdrs["To"]
	out.Cc = hdrs["Cc"]
	if b, err := json.Marshal(hdrs); err == nil {
		out.HeadersJSON = string(b)
	}
	return out, nil
}

type part struct {
	MimeType string `json:"mimeType"`
	Filename string `json:"filename"`
	Headers  []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"headers"`
	Body struct {
		Data         string `json:"data"`
		AttachmentID string `json:"attachmentId"`
		Size         int64  `json:"size"`
	} `json:"body"`
	Parts []part `json:"parts"`
}

func walkParts(parts []part, rootMime, rootData, want string) string {
	if rootMime == want && rootData != "" {
		if b, err := base64.URLEncoding.DecodeString(rootData); err == nil {
			return string(b)
		}
	}
	for _, p := range parts {
		if p.MimeType == want {
			if b, err := base64.URLEncoding.DecodeString(p.Body.Data); err == nil {
				return string(b)
			}
		}
		if nested := walkParts(p.Parts, "", "", want); nested != "" {
			return nested
		}
	}
	return ""
}

// gmailAttachment is one inline-image or file attachment ready to persist.
type gmailAttachment struct {
	ContentID    string
	Filename     string
	MIME         string
	Inline       bool
	Data         []byte
	AttachmentID string // set when Gmail deferred the body — hydrate via attachments.get
}

// collectAttachments walks the part tree and returns every part that
// has a Content-ID header (inline image) or a non-empty filename
// (regular attachment). Body data may still be empty when Gmail decided
// the part was big enough to require a follow-up attachments.get fetch;
// callers should hydrate those via fetchAttachmentData.
func collectAttachments(parts []part) []gmailAttachment {
	var out []gmailAttachment
	var walk func([]part)
	walk = func(ps []part) {
		for _, p := range ps {
			if hasAttachment(p) {
				out = append(out, gmailAttachment{
					ContentID:    partHeader(p, "Content-ID"),
					Filename:     p.Filename,
					MIME:         p.MimeType,
					Inline:       isInlinePart(p),
					Data:         decodeURL(p.Body.Data),
					AttachmentID: p.Body.AttachmentID,
				})
			}
			if len(p.Parts) > 0 {
				walk(p.Parts)
			}
		}
	}
	walk(parts)
	return out
}

func hasAttachment(p part) bool {
	if p.Filename != "" {
		return true
	}
	if partHeader(p, "Content-ID") != "" {
		return true
	}
	if strings.HasPrefix(p.MimeType, "image/") && p.Body.AttachmentID != "" {
		return true
	}
	return false
}

func isInlinePart(p part) bool {
	if partHeader(p, "Content-ID") != "" {
		return true
	}
	cd := strings.ToLower(partHeader(p, "Content-Disposition"))
	return strings.HasPrefix(cd, "inline")
}

func partHeader(p part, name string) string {
	for _, h := range p.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

func decodeURL(s string) []byte {
	if s == "" {
		return nil
	}
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return nil
	}
	return b
}

// fetchAttachmentData hydrates a part whose body was deferred. Gmail's
// REST API returns bodies inline for small parts; large ones (typically
// >~5MB) come back as just an attachmentId reference.
func fetchAttachmentData(ctx context.Context, client *http.Client, msgID, attachmentID string) ([]byte, error) {
	u := fmt.Sprintf(
		"https://gmail.googleapis.com/gmail/v1/users/me/messages/%s/attachments/%s",
		url.PathEscape(msgID), url.PathEscape(attachmentID),
	)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("attachments.get: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("attachments.get: %s", resp.Status)
	}
	var body struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return decodeURL(body.Data), nil
}

func internalDateToRFC3339(ms string) string {
	if ms == "" {
		return ""
	}
	var n int64
	for _, r := range ms {
		if r < '0' || r > '9' {
			return ""
		}
		n = n*10 + int64(r-'0')
	}
	return time.UnixMilli(n).UTC().Format(time.RFC3339)
}

// splitAddress turns "Name <user@host>" into ("Name", "user@host").
func splitAddress(raw string) (name, addr string) {
	raw = strings.TrimSpace(raw)
	if i := strings.LastIndex(raw, "<"); i >= 0 && strings.HasSuffix(raw, ">") {
		name = strings.Trim(strings.TrimSpace(raw[:i]), `"`)
		addr = strings.TrimSuffix(raw[i+1:], ">")
		return
	}
	addr = raw
	return
}

// secretRefFor must match account.secretRef but lives here to avoid an
// import cycle.
func secretRefFor(email string) string { return "gmail_oauth:" + email }

// categoryFromLabels maps Gmail's CATEGORY_* system labels into our enum.
// First match wins; returns "" if no match so the local rules engine can
// take over for non-Gmail-categorised mail.
func categoryFromLabels(labels []string) string {
	for _, l := range labels {
		switch l {
		case "CATEGORY_PROMOTIONS":
			return "promotions"
		case "CATEGORY_SOCIAL":
			return "social"
		case "CATEGORY_UPDATES":
			return "updates"
		case "CATEGORY_FORUMS":
			return "newsletter"
		}
	}
	return ""
}
