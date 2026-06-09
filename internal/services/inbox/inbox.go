// Package inbox exposes read-side email queries to the frontend.
// Write paths (ingest, summarisation) belong to their respective services.
package inbox

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mandloideep/miniclaw/internal/db/sqlcgen"
)

// Email is the frontend-facing row.
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

// Service is the Wails-registered service.
type Service struct {
	q *sqlcgen.Queries
}

// New wires the service.
func New(pool *sql.DB) *Service { return &Service{q: sqlcgen.New(pool)} }

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

// MarkRead flips the read bit on.
func (s *Service) MarkRead(ctx context.Context, id int64) error {
	return s.q.MarkEmailRead(ctx, id)
}

// MarkUnread flips the read bit off.
func (s *Service) MarkUnread(ctx context.Context, id int64) error {
	return s.q.MarkEmailUnread(ctx, id)
}
