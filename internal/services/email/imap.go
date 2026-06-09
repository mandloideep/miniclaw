// Package email handles IMAP/SMTP ingest and (later) Gmail OAuth.
//
// IMAPSync connects to a single account's mailbox, walks unseen messages in
// INBOX since the configured fetch window (or since the highest UID we've
// already stored), and upserts them into the emails table. Dedup is on
// (account_id, message_id) via the schema's UNIQUE constraint plus an
// upsert in the query, so re-running is safe and cheap.
package email

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/mail"

	"github.com/mandloideep/miniclaw/internal/db/sqlcgen"
	"github.com/mandloideep/miniclaw/internal/services/account"
	"github.com/mandloideep/miniclaw/internal/services/triage"
)

// Classifier produces a category label for a stored email. Wired to
// internal/services/categories.Classify in main; nil-safe inside Sync.
type Classifier func(fromAddress, listUnsubscribeHeader string) string

// IMAPSyncer ingests one account at a time.
type IMAPSyncer struct {
	q        *sqlcgen.Queries
	accounts *account.Service
	triage   *triage.Service
	classify Classifier
	// dialTLS is overridable for tests.
	dialTLS func(addr string, opts *imapclient.Options) (*imapclient.Client, error)
}

// NewIMAPSyncer wires the syncer to the shared DB pool and account service.
// triage and classify may both be nil — sync skips the corresponding step
// when absent (useful for tests that don't need the full pipeline).
func NewIMAPSyncer(pool *sql.DB, acc *account.Service, tri *triage.Service, classify Classifier) *IMAPSyncer {
	return &IMAPSyncer{
		q:        sqlcgen.New(pool),
		accounts: acc,
		triage:   tri,
		classify: classify,
		dialTLS:  imapclient.DialTLS,
	}
}

// Sync runs one pass over account id's INBOX, upserting every message
// received since the account's fetch_since (or since the highest UID we
// already have in INBOX, whichever cuts more). Returns the count of new or
// updated rows.
func (s *IMAPSyncer) Sync(ctx context.Context, accountID int64) (int, error) {
	acc, err := s.accounts.Get(ctx, accountID)
	if err != nil {
		return 0, err
	}
	if acc.AuthKind != account.AuthIMAP {
		return 0, fmt.Errorf("account %d is not IMAP", accountID)
	}
	pwd, err := s.accounts.Password(ctx, accountID)
	if err != nil {
		return 0, fmt.Errorf("load password: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", acc.IMAPHost, acc.IMAPPort)
	c, err := s.dialTLS(addr, &imapclient.Options{
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: acc.IMAPHost},
	})
	if err != nil {
		return 0, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = c.Close() }()

	if err := c.Login(acc.EmailAddress, pwd).Wait(); err != nil {
		return 0, fmt.Errorf("login: %w", err)
	}
	defer func() { _ = c.Logout().Wait() }()

	total := 0
	for _, folder := range pickFolders(acc.FolderAllowlist) {
		n, err := s.syncFolder(ctx, c, accountID, folder, acc.FetchSince)
		if err != nil {
			// Don't abort the whole pass on one folder failing — surface and continue.
			fmt.Printf("imap sync: account %d folder %s: %v\n", accountID, folder, err)
			continue
		}
		total += n
	}
	if err := s.accounts.MarkSynced(ctx, accountID); err != nil {
		return total, fmt.Errorf("mark synced: %w", err)
	}
	return total, nil
}

// pickFolders parses the per-account allowlist CSV. Empty means INBOX only,
// which is the safe default for any provider.
func pickFolders(csv string) []string {
	if csv == "" {
		return []string{"INBOX"}
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		f := strings.TrimSpace(p)
		if f != "" && !seen[f] {
			out = append(out, f)
			seen[f] = true
		}
	}
	if len(out) == 0 {
		return []string{"INBOX"}
	}
	return out
}

// syncFolder selects a folder on an already-logged-in connection and
// upserts every message above the stored UID watermark for that folder.
func (s *IMAPSyncer) syncFolder(ctx context.Context, c *imapclient.Client, accountID int64, folder, fetchSince string) (int, error) {
	if _, err := c.Select(folder, &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		return 0, fmt.Errorf("select %s: %w", folder, err)
	}

	criteria := &imap.SearchCriteria{}
	if fetchSince != "" {
		t, perr := parseFetchSince(fetchSince)
		if perr != nil {
			return 0, perr
		}
		criteria.Since = t
	}
	maxUID, err := s.q.MaxUIDForFolder(ctx, sqlcgen.MaxUIDForFolderParams{
		AccountID: accountID, Folder: folder,
	})
	if err != nil {
		return 0, fmt.Errorf("read max uid: %w", err)
	}
	if maxUID > 0 {
		next := maxUID + 1
		if next < 0 || next > 0xFFFFFFFF {
			next = 0xFFFFFFFF
		}
		criteria.UID = []imap.UIDSet{{{Start: imap.UID(uint32(next)), Stop: 0}}}
	}

	searchData, err := c.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return 0, fmt.Errorf("uid search: %w", err)
	}
	uids := searchData.AllUIDs()
	if len(uids) == 0 {
		return 0, nil
	}

	uidSet := imap.UIDSet{}
	for _, u := range uids {
		uidSet.AddNum(u)
	}
	bodySection := &imap.FetchItemBodySection{}
	fetchOpts := &imap.FetchOptions{
		UID:         true,
		Envelope:    true,
		Flags:       true,
		BodySection: []*imap.FetchItemBodySection{bodySection},
	}
	msgs, err := c.Fetch(uidSet, fetchOpts).Collect()
	if err != nil {
		return 0, fmt.Errorf("fetch: %w", err)
	}

	written := 0
	for _, msg := range msgs {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		raw := msg.FindBodySection(bodySection)
		plain, htmlBody, headers := parseMessage(raw)

		hdrJSON, _ := json.Marshal(headers)
		from := firstAddress(msg.Envelope.From)
		to := joinAddresses(msg.Envelope.To)
		cc := joinAddresses(msg.Envelope.Cc)

		if s.triage != nil {
			blocked, blockErr := s.triage.MatchesBlock(ctx, accountID, from.address)
			if blockErr == nil && blocked {
				continue
			}
		}
		if s.triage != nil {
			_ = s.triage.RegisterSender(ctx, accountID, from.address)
		}

		id, err := s.q.UpsertEmail(ctx, sqlcgen.UpsertEmailParams{
			AccountID:   accountID,
			MessageID:   msg.Envelope.MessageID,
			Folder:      folder,
			Uid:         sql.NullInt64{Int64: int64(msg.UID), Valid: true},
			FromAddress: from.address,
			FromName:    from.name,
			ToAddresses: to,
			CcAddresses: cc,
			Subject:     msg.Envelope.Subject,
			ReceivedAt:  msg.Envelope.Date.UTC().Format(time.RFC3339),
			BodyPlain:   plain,
			BodyHtml:    htmlBody,
			HeadersJson: string(hdrJSON),
		})
		if err != nil {
			return written, fmt.Errorf("upsert message %s: %w", msg.Envelope.MessageID, err)
		}
		if s.classify != nil {
			if cat := s.classify(from.address, headers["List-Unsubscribe"]); cat != "" {
				_ = s.q.SetCategory(ctx, sqlcgen.SetCategoryParams{Category: cat, ID: id})
			}
		}
		written++
	}
	return written, nil
}

// parseFetchSince accepts ISO date (YYYY-MM-DD) or RFC3339 timestamps.
func parseFetchSince(v string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("fetch_since %q: not RFC3339 or YYYY-MM-DD", v)
}

type addr struct{ name, address string }

func firstAddress(in []imap.Address) addr {
	if len(in) == 0 {
		return addr{}
	}
	return addr{name: in[0].Name, address: fmt.Sprintf("%s@%s", in[0].Mailbox, in[0].Host)}
}

func joinAddresses(in []imap.Address) string {
	if len(in) == 0 {
		return ""
	}
	parts := make([]string, 0, len(in))
	for _, a := range in {
		parts = append(parts, fmt.Sprintf("%s@%s", a.Mailbox, a.Host))
	}
	return strings.Join(parts, ", ")
}

// parseMessage walks the raw RFC822 bytes and pulls out text/plain, text/html
// (best-effort), and a flattened header map.
func parseMessage(raw []byte) (plain, htmlBody string, headers map[string]string) {
	headers = map[string]string{}
	if len(raw) == 0 {
		return
	}
	mr, err := mail.CreateReader(strings.NewReader(string(raw)))
	if err != nil {
		// Fall back: dump everything as plain text. Better some content
		// than none when a server hands us something quirky.
		plain = string(raw)
		return
	}
	for _, key := range []string{"From", "To", "Cc", "Subject", "Date", "Message-ID", "List-Unsubscribe"} {
		if v := mr.Header.Get(key); v != "" {
			headers[key] = v
		}
	}
	for {
		p, perr := mr.NextPart()
		if errors.Is(perr, io.EOF) {
			break
		}
		if perr != nil {
			break
		}
		body, _ := io.ReadAll(p.Body)
		if h, ok := p.Header.(*mail.InlineHeader); ok {
			ct, _, _ := h.ContentType()
			switch ct {
			case "text/plain":
				if plain == "" {
					plain = string(body)
				}
			case "text/html":
				if htmlBody == "" {
					htmlBody = string(body)
				}
			}
		}
	}
	if plain == "" && htmlBody != "" {
		// Strip a poor-man's plaintext from HTML if no text/plain part exists.
		plain = stripHTML(htmlBody)
	}
	return
}

// stripHTML is a deliberately dumb tag remover used only as a last-resort
// summariser feed when text/plain is missing. It's not safe HTML rendering.
func stripHTML(in string) string {
	var b strings.Builder
	inside := false
	for _, r := range in {
		switch {
		case r == '<':
			inside = true
		case r == '>':
			inside = false
		case !inside:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
