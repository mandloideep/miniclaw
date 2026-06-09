// Package inbox exposes read-side email queries to the frontend.
// Write paths (ingest, summarisation) belong to their respective services.
package inbox

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mandloideep/miniclaw/internal/db/sqlcgen"
)

// Email is the list-row shape used by the inbox view.
type Email struct {
	ID          int64  `json:"id"`
	AccountID   int64  `json:"accountId"`
	FromAddress string `json:"fromAddress"`
	FromName    string `json:"fromName"`
	Subject     string `json:"subject"`
	ReceivedAt  string `json:"receivedAt"`
	BodyPlain   string `json:"bodyPlain"`
	IsRead      bool   `json:"isRead"`
	IsPutAside  bool   `json:"isPutAside"`
	Category    string `json:"category"`
}

// EmailDetail is the reader-pane shape. Includes HTML, recipients, and a
// best-effort header map so the UI can render rich mail.
type EmailDetail struct {
	Email
	To       string `json:"to"`
	Cc       string `json:"cc"`
	BodyHTML string `json:"bodyHtml"`
	Headers  string `json:"headers"`
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
			Category: r.Category,
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
			Category: r.Category,
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
		SELECT id, account_id, from_address, from_name, to_addresses, cc_addresses,
		       subject, received_at, body_plain, body_html, headers_json,
		       is_read, is_put_aside, category
		FROM emails WHERE id = ? LIMIT 1`
	var d EmailDetail
	var isRead, isPutAside int64
	row := s.pool.QueryRowContext(ctx, q, id)
	if err := row.Scan(
		&d.ID, &d.AccountID, &d.FromAddress, &d.FromName, &d.To, &d.Cc,
		&d.Subject, &d.ReceivedAt, &d.BodyPlain, &d.BodyHTML, &d.Headers,
		&isRead, &isPutAside, &d.Category,
	); err != nil {
		return EmailDetail{}, fmt.Errorf("get email %d: %w", id, err)
	}
	d.IsRead = isRead == 1
	d.IsPutAside = isPutAside == 1
	return d, nil
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
		       e.received_at, e.body_plain, e.is_read, e.is_put_aside, e.category
		FROM emails e
		JOIN accounts a ON a.id = e.account_id
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
		var isRead, isPutAside int64
		if err := rows.Scan(
			&e.ID, &e.AccountID, &e.FromAddress, &e.FromName,
			&e.Subject, &e.ReceivedAt, &e.BodyPlain,
			&isRead, &isPutAside, &e.Category,
		); err != nil {
			return nil, fmt.Errorf("scan older email: %w", err)
		}
		e.IsRead = isRead == 1
		e.IsPutAside = isPutAside == 1
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
