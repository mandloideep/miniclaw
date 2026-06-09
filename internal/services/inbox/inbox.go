// Package inbox exposes read-side email queries to the frontend.
// Write paths (ingest, summarisation) belong to their respective services.
package inbox

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/mandloideep/miniclaw/internal/db/sqlcgen"
)

// Email is the list-row shape used by the inbox view.
type Email struct {
	ID              int64  `json:"id"`
	AccountID       int64  `json:"accountId"`
	FromAddress     string `json:"fromAddress"`
	FromName        string `json:"fromName"`
	Subject         string `json:"subject"`
	ReceivedAt      string `json:"receivedAt"`
	BodyPlain       string `json:"bodyPlain"`
	IsRead          bool   `json:"isRead"`
	IsPutAside      bool   `json:"isPutAside"`
	Category        string `json:"category"`
	Summary         string `json:"summary,omitempty"`
	NeedsAttention  bool   `json:"needsAttention,omitempty"`
	AttentionReason string `json:"attentionReason,omitempty"`
}

// EmailDetail is the reader-pane shape. Includes HTML, recipients, and a
// best-effort header map so the UI can render rich mail.
type EmailDetail struct {
	Email
	To       string   `json:"to"`
	Cc       string   `json:"cc"`
	BodyHTML string   `json:"bodyHtml"`
	Headers  string   `json:"headers"`
	Labels   []string `json:"labels"`
}

// Service is the Wails-registered service.
type Service struct {
	q    *sqlcgen.Queries
	pool *sql.DB
}

// New wires the service.
func New(pool *sql.DB) *Service { return &Service{q: sqlcgen.New(pool), pool: pool} }

// ListByWorkspace returns the most recent N emails in a workspace.
func (s *Service) ListByWorkspace(ctx context.Context, workspaceID, limit int64) ([]Email, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.q.ListEmailsByWorkspace(ctx, sqlcgen.ListEmailsByWorkspaceParams{
		WorkspaceID: workspaceID,
		Limit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list emails by workspace: %w", err)
	}
	out := make([]Email, 0, len(rows))
	for _, r := range rows {
		out = append(out, Email{
			ID: r.ID, AccountID: r.AccountID,
			FromAddress: r.FromAddress, FromName: r.FromName,
			Subject: r.Subject, ReceivedAt: r.ReceivedAt,
			BodyPlain: r.BodyPlain,
			IsRead:    r.IsRead == 1, IsPutAside: r.IsPutAside == 1,
			Category:        r.Category,
			Summary:         r.Summary,
			NeedsAttention:  r.NeedsAttention == 1,
			AttentionReason: r.AttentionReason,
		})
	}
	return out, nil
}

// ListByAccount returns the most recent N emails in a single account.
func (s *Service) ListByAccount(ctx context.Context, accountID, limit int64) ([]Email, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.q.ListEmailsByAccount(ctx, sqlcgen.ListEmailsByAccountParams{
		AccountID: accountID,
		Limit:     limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list emails by account: %w", err)
	}
	out := make([]Email, 0, len(rows))
	for _, r := range rows {
		out = append(out, Email{
			ID: r.ID, AccountID: r.AccountID,
			FromAddress: r.FromAddress, FromName: r.FromName,
			Subject: r.Subject, ReceivedAt: r.ReceivedAt,
			BodyPlain: r.BodyPlain,
			IsRead:    r.IsRead == 1, IsPutAside: r.IsPutAside == 1,
			Category:        r.Category,
			Summary:         r.Summary,
			NeedsAttention:  r.NeedsAttention == 1,
			AttentionReason: r.AttentionReason,
		})
	}
	return out, nil
}

// Get returns one email with the full reader-pane payload (HTML body,
// recipients, header map). Direct SQL because sqlc doesn't have a
// matching SELECT yet and regenerating during a UI iteration is more
// churn than it's worth.
func (s *Service) Get(ctx context.Context, id int64) (EmailDetail, error) {
	const q = `
		SELECT e.id, e.account_id, e.from_address, e.from_name, e.to_addresses, e.cc_addresses,
		       e.subject, e.received_at, e.body_plain, e.body_html, e.headers_json,
		       e.is_read, e.is_put_aside, e.category,
		       COALESCE(s.summary, ''),
		       COALESCE(s.needs_attention, 0),
		       COALESCE(s.attention_reason, '')
		FROM emails e
		LEFT JOIN summaries s ON s.email_id = e.id
		WHERE e.id = ? LIMIT 1`
	var d EmailDetail
	var isRead, isPutAside, needs int64
	row := s.pool.QueryRowContext(ctx, q, id)
	if err := row.Scan(
		&d.ID, &d.AccountID, &d.FromAddress, &d.FromName, &d.To, &d.Cc,
		&d.Subject, &d.ReceivedAt, &d.BodyPlain, &d.BodyHTML, &d.Headers,
		&isRead, &isPutAside, &d.Category,
		&d.Summary, &needs, &d.AttentionReason,
	); err != nil {
		return EmailDetail{}, fmt.Errorf("get email %d: %w", id, err)
	}
	d.IsRead = isRead == 1
	d.IsPutAside = isPutAside == 1
	d.NeedsAttention = needs == 1
	labels, lerr := s.q.ListEmailLabels(ctx, id)
	if lerr == nil {
		d.Labels = labels
	} else {
		d.Labels = []string{}
	}
	return d, nil
}

// ListLabels returns the label strings attached to one email. Exposed
// separately so list rows can decorate themselves without loading the
// full reader payload.
func (s *Service) ListLabels(ctx context.Context, emailID int64) ([]string, error) {
	labels, err := s.q.ListEmailLabels(ctx, emailID)
	if err != nil {
		return nil, fmt.Errorf("list email labels: %w", err)
	}
	if labels == nil {
		labels = []string{}
	}
	return labels, nil
}

// ListOlderByWorkspace returns N emails strictly older than beforeReceivedAt
// (RFC3339). Empty beforeReceivedAt falls back to ListByWorkspace. Used by
// the frontend for "load older" infinite scroll.
func (s *Service) ListOlderByWorkspace(ctx context.Context, workspaceID int64, beforeReceivedAt string, limit int64) ([]Email, error) {
	if beforeReceivedAt == "" {
		return s.ListByWorkspace(ctx, workspaceID, limit)
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	const q = `
		SELECT e.id, e.account_id, e.from_address, e.from_name, e.subject,
		       e.received_at, e.body_plain, e.is_read, e.is_put_aside, e.category,
		       COALESCE(s.summary, ''),
		       COALESCE(s.needs_attention, 0),
		       COALESCE(s.attention_reason, '')
		FROM emails e
		JOIN accounts a ON a.id = e.account_id
		LEFT JOIN summaries s ON s.email_id = e.id
		WHERE a.workspace_id = ? AND e.received_at < ?
		ORDER BY e.received_at DESC
		LIMIT ?`
	rows, err := s.pool.QueryContext(ctx, q, workspaceID, beforeReceivedAt, limit)
	if err != nil {
		return nil, fmt.Errorf("list older emails: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]Email, 0, int(limit))
	for rows.Next() {
		var e Email
		var isRead, isPutAside, needs int64
		if err := rows.Scan(
			&e.ID, &e.AccountID, &e.FromAddress, &e.FromName,
			&e.Subject, &e.ReceivedAt, &e.BodyPlain,
			&isRead, &isPutAside, &e.Category,
			&e.Summary, &needs, &e.AttentionReason,
		); err != nil {
			return nil, fmt.Errorf("scan older email: %w", err)
		}
		e.IsRead = isRead == 1
		e.IsPutAside = isPutAside == 1
		e.NeedsAttention = needs == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// Search runs the FTS5 query against the emails_fts virtual table for
// emails in a workspace. q is a raw FTS5 match expression — the
// frontend should treat user input as a single phrase ("…") unless the
// user explicitly uses operators.
func (s *Service) Search(ctx context.Context, workspaceID int64, q string, limit int64) ([]Email, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return s.ListByWorkspace(ctx, workspaceID, limit)
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	// Wrap in quotes so user input is matched as a phrase, escape "" inside.
	phrase := `"` + strings.ReplaceAll(q, `"`, `""`) + `"`
	const sqlQ = `
SELECT e.id, e.account_id, e.from_address, e.from_name, e.subject,
       e.received_at, e.body_plain, e.is_read, e.is_put_aside, e.category,
       COALESCE(sm.summary, ''),
       COALESCE(sm.needs_attention, 0),
       COALESCE(sm.attention_reason, '')
FROM emails_fts f
JOIN emails e ON e.id = f.rowid
JOIN accounts a ON a.id = e.account_id
LEFT JOIN summaries sm ON sm.email_id = e.id
WHERE a.workspace_id = ? AND emails_fts MATCH ?
ORDER BY e.received_at DESC
LIMIT ?`
	rows, err := s.pool.QueryContext(ctx, sqlQ, workspaceID, phrase, limit)
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]Email, 0, int(limit))
	for rows.Next() {
		var e Email
		var isRead, isPutAside, needs int64
		if err := rows.Scan(
			&e.ID, &e.AccountID, &e.FromAddress, &e.FromName,
			&e.Subject, &e.ReceivedAt, &e.BodyPlain,
			&isRead, &isPutAside, &e.Category,
			&e.Summary, &needs, &e.AttentionReason,
		); err != nil {
			return nil, fmt.Errorf("scan search row: %w", err)
		}
		e.IsRead = isRead == 1
		e.IsPutAside = isPutAside == 1
		e.NeedsAttention = needs == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkRead flips the read bit on.
func (s *Service) MarkRead(ctx context.Context, id int64) error {
	return s.q.MarkEmailRead(ctx, id)
}

// MarkUnread flips the read bit off.
func (s *Service) MarkUnread(ctx context.Context, id int64) error {
	return s.q.MarkEmailUnread(ctx, id)
}
