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

// Syncer ingests one Gmail-OAuth account at a time using the REST API.
type Syncer struct {
	q        *sqlcgen.Queries
	accounts *account.Service
}

// NewSyncer wires the syncer to the shared pool and account service.
func NewSyncer(pool *sql.DB, acc *account.Service) *Syncer {
	return &Syncer{q: sqlcgen.New(pool), accounts: acc}
}

// Sync pulls messages from the user's INBOX newer than the account's
// last_synced_at (or fetch_since if that's later). Returns count written.
//
// Uses /gmail/v1/users/me/messages with a Gmail search query, then
// /gmail/v1/users/me/messages/{id}?format=full for each. Dedupe is on
// (account_id, message_id) via the schema's UNIQUE constraint.
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

	q := gmailWatermarkQuery(acc.FetchSince, acc.LastSyncedAt)
	ids, err := listMessageIDs(ctx, client, q)
	if err != nil {
		return 0, err
	}
	written := 0
	for _, id := range ids {
		if cerr := ctx.Err(); cerr != nil {
			return written, cerr
		}
		msg, ferr := fetchMessage(ctx, client, id)
		if ferr != nil {
			continue
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
		})
		if err == nil {
			if msg.Category != "" {
				_ = s.q.SetCategory(ctx, sqlcgen.SetCategoryParams{
					Category: msg.Category, ID: id2,
				})
			}
			written++
		}
	}
	if err := s.accounts.MarkSynced(ctx, accountID); err != nil {
		return written, err
	}
	return written, nil
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
	u := "https://gmail.googleapis.com/gmail/v1/users/me/messages?q=" + url.QueryEscape(q) + "&maxResults=200"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("messages.list: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("messages.list: %s", resp.Status)
	}
	var body struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(body.Messages))
	for _, m := range body.Messages {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// gmailMessage is what fetchMessage returns once the REST payload is parsed.
type gmailMessage struct {
	MessageID   string
	FromAddress string
	FromName    string
	To, Cc      string
	Subject     string
	ReceivedAt  string
	BodyPlain   string
	BodyHTML    string
	HeadersJSON string
	Category    string // mapped from Gmail labelIds; empty if not categorisable
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
		MessageID:  hdrs["Message-ID"],
		Subject:    hdrs["Subject"],
		ReceivedAt: internalDateToRFC3339(body.InternalDate),
		BodyPlain:  walkParts(body.Payload.Parts, body.Payload.MimeType, body.Payload.Body.Data, "text/plain"),
		BodyHTML:   walkParts(body.Payload.Parts, body.Payload.MimeType, body.Payload.Body.Data, "text/html"),
		Category:   categoryFromLabels(body.LabelIds),
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
	Body     struct {
		Data string `json:"data"`
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
