package msoauth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/mandloideep/miniclaw/internal/db/sqlcgen"
	"github.com/mandloideep/miniclaw/internal/services/account"
)

// Syncer ingests one Microsoft-OAuth account at a time using Graph.
type Syncer struct {
	q        *sqlcgen.Queries
	accounts *account.Service
}

// NewSyncer wires the syncer.
func NewSyncer(pool *sql.DB, acc *account.Service) *Syncer {
	return &Syncer{q: sqlcgen.New(pool), accounts: acc}
}

// Sync pulls messages from Inbox newer than the watermark.
func (s *Syncer) Sync(ctx context.Context, accountID int64) (int, error) {
	acc, err := s.accounts.Get(ctx, accountID)
	if err != nil {
		return 0, err
	}
	if acc.AuthKind != account.AuthMSOAuth {
		return 0, fmt.Errorf("account %d is not MS OAuth", accountID)
	}
	src, err := TokenSource(ctx, secretRefFor(acc.EmailAddress))
	if err != nil {
		return 0, err
	}
	client := oauth2.NewClient(ctx, src)

	since := watermark(acc.FetchSince, acc.LastSyncedAt)
	msgs, err := listMessages(ctx, client, since)
	if err != nil {
		return 0, err
	}
	written := 0
	for _, m := range msgs {
		if cerr := ctx.Err(); cerr != nil {
			return written, cerr
		}
		_, err := s.q.UpsertEmail(ctx, sqlcgen.UpsertEmailParams{
			AccountID:   accountID,
			MessageID:   m.MessageID,
			Folder:      "INBOX",
			Uid:         sql.NullInt64{},
			FromAddress: m.FromAddress,
			FromName:    m.FromName,
			ToAddresses: m.To,
			CcAddresses: m.Cc,
			Subject:     m.Subject,
			ReceivedAt:  m.ReceivedAt,
			BodyPlain:   m.BodyPlain,
			BodyHtml:    m.BodyHTML,
			HeadersJson: m.HeadersJSON,
		})
		if err == nil {
			written++
		}
	}
	if err := s.accounts.MarkSynced(ctx, accountID); err != nil {
		return written, err
	}
	return written, nil
}

// watermark picks the more restrictive of fetchSince and lastSyncedAt and
// returns an RFC3339 UTC string Graph accepts via $filter receivedDateTime ge.
func watermark(fetchSince, lastSynced string) string {
	var newest time.Time
	for _, v := range []string{fetchSince, lastSynced} {
		if v == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			t, err = time.Parse("2006-01-02", v)
		}
		if err == nil && t.After(newest) {
			newest = t
		}
	}
	if newest.IsZero() {
		return ""
	}
	return newest.UTC().Format(time.RFC3339)
}

type graphMessage struct {
	MessageID   string
	FromAddress string
	FromName    string
	To, Cc      string
	Subject     string
	ReceivedAt  string
	BodyPlain   string
	BodyHTML    string
	HeadersJSON string
}

func listMessages(ctx context.Context, client *http.Client, since string) ([]graphMessage, error) {
	// $select keeps the payload small; ask for body separately if needed.
	q := url.Values{}
	q.Set("$top", "100")
	q.Set("$orderby", "receivedDateTime desc")
	q.Set("$select", "id,internetMessageId,subject,from,toRecipients,ccRecipients,receivedDateTime,bodyPreview,body,internetMessageHeaders")
	if since != "" {
		q.Set("$filter", fmt.Sprintf("receivedDateTime ge %s", since))
	}
	u := "https://graph.microsoft.com/v1.0/me/mailFolders/Inbox/messages?" + q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("messages list: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("messages list: %s", resp.Status)
	}
	var body struct {
		Value []rawMessage `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]graphMessage, 0, len(body.Value))
	for _, r := range body.Value {
		out = append(out, fromRaw(r))
	}
	return out, nil
}

type rawAddress struct {
	EmailAddress struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	} `json:"emailAddress"`
}

type rawMessage struct {
	ID                string       `json:"id"`
	InternetMessageID string       `json:"internetMessageId"`
	Subject           string       `json:"subject"`
	ReceivedDateTime  string       `json:"receivedDateTime"`
	From              rawAddress   `json:"from"`
	ToRecipients      []rawAddress `json:"toRecipients"`
	CcRecipients      []rawAddress `json:"ccRecipients"`
	Body              struct {
		ContentType string `json:"contentType"`
		Content     string `json:"content"`
	} `json:"body"`
	InternetMessageHeaders []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"internetMessageHeaders"`
}

func fromRaw(r rawMessage) graphMessage {
	msg := graphMessage{
		MessageID:   firstNonEmpty(r.InternetMessageID, r.ID),
		FromName:    r.From.EmailAddress.Name,
		FromAddress: r.From.EmailAddress.Address,
		To:          joinAddrs(r.ToRecipients),
		Cc:          joinAddrs(r.CcRecipients),
		Subject:     r.Subject,
		ReceivedAt:  r.ReceivedDateTime,
	}
	if strings.EqualFold(r.Body.ContentType, "text") {
		msg.BodyPlain = r.Body.Content
	} else {
		msg.BodyHTML = r.Body.Content
	}
	hdrs := map[string]string{}
	for _, h := range r.InternetMessageHeaders {
		hdrs[h.Name] = h.Value
	}
	if b, err := json.Marshal(hdrs); err == nil {
		msg.HeadersJSON = string(b)
	}
	return msg
}

func joinAddrs(addrs []rawAddress) string {
	parts := make([]string, 0, len(addrs))
	for _, a := range addrs {
		parts = append(parts, a.EmailAddress.Address)
	}
	return strings.Join(parts, ", ")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// secretRefFor mirrors account.secretRef to avoid an import cycle.
func secretRefFor(email string) string { return "ms_oauth:" + email }
