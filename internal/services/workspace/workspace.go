// Package workspace exposes workspace CRUD to the frontend. A workspace is
// the "Family / Work / Personal / Other"-style grouping that owns accounts
// and gets fanned out to Telegram recipients.
package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mandloideep/miniclaw/internal/db/sqlcgen"
)

// Workspace is the frontend-facing shape. We don't re-export the sqlc type
// directly so the Wails binding generator sees a stable, hand-curated struct.
type Workspace struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Emoji     string `json:"emoji"`
	SortOrder int64  `json:"sortOrder"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Service is the Wails-registered service.
type Service struct {
	q *sqlcgen.Queries
}

// New takes the shared *sql.DB pool and returns a ready-to-register Service.
func New(pool *sql.DB) *Service {
	return &Service{q: sqlcgen.New(pool)}
}

// List returns all workspaces ordered by sort_order, id.
func (s *Service) List(ctx context.Context) ([]Workspace, error) {
	rows, err := s.q.ListWorkspaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	out := make([]Workspace, 0, len(rows))
	for _, r := range rows {
		out = append(out, toWorkspace(r))
	}
	return out, nil
}

// Get returns a single workspace by ID.
func (s *Service) Get(ctx context.Context, id int64) (Workspace, error) {
	r, err := s.q.GetWorkspace(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Workspace{}, fmt.Errorf("workspace %d: not found", id)
	}
	if err != nil {
		return Workspace{}, fmt.Errorf("get workspace %d: %w", id, err)
	}
	return toWorkspace(r), nil
}

// Create adds a new workspace with the given name and emoji.
func (s *Service) Create(ctx context.Context, name, emoji string) (Workspace, error) {
	if name == "" {
		return Workspace{}, errors.New("workspace name is required")
	}
	r, err := s.q.CreateWorkspace(ctx, sqlcgen.CreateWorkspaceParams{Name: name, Emoji: emoji})
	if err != nil {
		return Workspace{}, fmt.Errorf("create workspace: %w", err)
	}
	return toWorkspace(r), nil
}

// Update renames or re-emojis a workspace in place.
func (s *Service) Update(ctx context.Context, id int64, name, emoji string) error {
	if name == "" {
		return errors.New("workspace name is required")
	}
	return s.q.UpdateWorkspace(ctx, sqlcgen.UpdateWorkspaceParams{
		ID: id, Name: name, Emoji: emoji,
	})
}

// Delete removes a workspace. Accounts in it cascade-delete (and their
// emails/summaries with them) per the schema.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.q.DeleteWorkspace(ctx, id)
}

// Reorder sets the workspace's sort_order. Caller is expected to send a
// dense sequence (0,1,2,...) after a drag-reorder in the UI.
func (s *Service) Reorder(ctx context.Context, id, sortOrder int64) error {
	return s.q.ReorderWorkspace(ctx, sqlcgen.ReorderWorkspaceParams{
		ID: id, SortOrder: sortOrder,
	})
}

func toWorkspace(r sqlcgen.Workspace) Workspace {
	return Workspace{
		ID:        r.ID,
		Name:      r.Name,
		Emoji:     r.Emoji,
		SortOrder: r.SortOrder,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}
