// Package planner is the catch-all home for the workspace-scoped extras
// that aren't email: calendar blocks, todos, and notes. Each lives on
// a separate Wails service so the binding generator can group the
// frontend imports sensibly.
//
// Calendar blocks are local-first. google_event_id stays empty until
// the user promotes a block; only promoted rows would 2-way-sync with
// Google Calendar. Today the Google path isn't implemented — only the
// schema, service surface, and Promote stub are in place.
package planner

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/mandloideep/miniclaw/internal/db/sqlcgen"
)

// CalendarService exposes time-blocking CRUD.
type CalendarService struct {
	q *sqlcgen.Queries
}

// NewCalendar wires the calendar service to the shared pool.
func NewCalendar(pool *sql.DB) *CalendarService {
	return &CalendarService{q: sqlcgen.New(pool)}
}

// Block is the frontend-facing shape.
type Block struct {
	ID            int64  `json:"id"`
	WorkspaceID   int64  `json:"workspaceId"`
	Title         string `json:"title"`
	Notes         string `json:"notes,omitempty"`
	StartAt       string `json:"startAt"` // RFC3339 UTC
	EndAt         string `json:"endAt"`   // RFC3339 UTC
	Kind          string `json:"kind"`    // block | meeting | focus
	GoogleEventID string `json:"googleEventId,omitempty"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// List returns blocks for a workspace whose end_at is in the future or
// up to a fortnight in the past. We trim aggressively because the UI
// only ever renders a short window and unbounded growth makes the JSON
// round-trip slower than it needs to be.
func (s *CalendarService) List(ctx context.Context, workspaceID int64) ([]Block, error) {
	since := time.Now().UTC().Add(-14 * 24 * time.Hour).Format(time.RFC3339)
	rows, err := s.q.ListCalendarBlocks(ctx, sqlcgen.ListCalendarBlocksParams{
		WorkspaceID: workspaceID, EndAt: since,
	})
	if err != nil {
		return nil, fmt.Errorf("list blocks: %w", err)
	}
	out := make([]Block, 0, len(rows))
	for _, r := range rows {
		out = append(out, blockFrom(r))
	}
	return out, nil
}

// Create inserts a new block. Returns the persisted row.
func (s *CalendarService) Create(ctx context.Context, workspaceID int64, title, notes, startRFC3339, endRFC3339, kind string) (Block, error) {
	if err := validateBlockTimes(startRFC3339, endRFC3339); err != nil {
		return Block{}, err
	}
	if kind == "" {
		kind = "block"
	}
	r, err := s.q.CreateCalendarBlock(ctx, sqlcgen.CreateCalendarBlockParams{
		WorkspaceID: workspaceID, Title: title, Notes: notes,
		StartAt: startRFC3339, EndAt: endRFC3339, Kind: kind,
	})
	if err != nil {
		return Block{}, fmt.Errorf("create block: %w", err)
	}
	return blockFrom(r), nil
}

// Update mutates an existing block.
func (s *CalendarService) Update(ctx context.Context, id int64, title, notes, startRFC3339, endRFC3339, kind string) error {
	if err := validateBlockTimes(startRFC3339, endRFC3339); err != nil {
		return err
	}
	return s.q.UpdateCalendarBlock(ctx, sqlcgen.UpdateCalendarBlockParams{
		Title: title, Notes: notes,
		StartAt: startRFC3339, EndAt: endRFC3339, Kind: kind, ID: id,
	})
}

// Delete removes the block. If a google_event_id is set, the caller is
// responsible for cleaning it up on the Google side — promotion lives
// elsewhere because it depends on the OAuth helper.
func (s *CalendarService) Delete(ctx context.Context, id int64) error {
	return s.q.DeleteCalendarBlock(ctx, id)
}

// Promote stamps a placeholder google_event_id so the UI can mark the
// row as "intended to sync"; full two-way sync against the Google API
// lands later. Returns nil even when the block doesn't exist — the
// frontend treats Promote as best-effort.
func (s *CalendarService) Promote(ctx context.Context, id int64) error {
	return s.q.SetCalendarGoogleEvent(ctx, sqlcgen.SetCalendarGoogleEventParams{
		GoogleEventID: fmt.Sprintf("pending:%d", id), ID: id,
	})
}

func validateBlockTimes(startRFC3339, endRFC3339 string) error {
	start, err := time.Parse(time.RFC3339, startRFC3339)
	if err != nil {
		return fmt.Errorf("start_at: %w", err)
	}
	end, err := time.Parse(time.RFC3339, endRFC3339)
	if err != nil {
		return fmt.Errorf("end_at: %w", err)
	}
	if !end.After(start) {
		return fmt.Errorf("end_at must be after start_at")
	}
	return nil
}

func blockFrom(r sqlcgen.CalendarBlock) Block {
	return Block{
		ID: r.ID, WorkspaceID: r.WorkspaceID,
		Title: r.Title, Notes: r.Notes,
		StartAt: r.StartAt, EndAt: r.EndAt, Kind: r.Kind,
		GoogleEventID: r.GoogleEventID,
		CreatedAt:     r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// TodosService exposes per-workspace todo CRUD.
type TodosService struct {
	q *sqlcgen.Queries
}

// NewTodos wires the todos service to the shared pool.
func NewTodos(pool *sql.DB) *TodosService { return &TodosService{q: sqlcgen.New(pool)} }

// Todo is the frontend-facing shape.
type Todo struct {
	ID          int64  `json:"id"`
	WorkspaceID int64  `json:"workspaceId"`
	Text        string `json:"text"`
	Done        bool   `json:"done"`
	DueAt       string `json:"dueAt,omitempty"`
	SortOrder   int64  `json:"sortOrder"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// List returns the todos for a workspace, open first then done.
func (s *TodosService) List(ctx context.Context, workspaceID int64) ([]Todo, error) {
	rows, err := s.q.ListTodos(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list todos: %w", err)
	}
	out := make([]Todo, 0, len(rows))
	for _, r := range rows {
		out = append(out, Todo{
			ID: r.ID, WorkspaceID: r.WorkspaceID,
			Text: r.Text, Done: r.Done == 1, DueAt: r.DueAt,
			SortOrder: r.SortOrder,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}
	return out, nil
}

// Create inserts a new open todo, dropping it at the end of the list.
func (s *TodosService) Create(ctx context.Context, workspaceID int64, text, dueAt string) (Todo, error) {
	if text == "" {
		return Todo{}, fmt.Errorf("text is required")
	}
	next, err := s.q.NextTodoSort(ctx, workspaceID)
	if err != nil {
		return Todo{}, fmt.Errorf("next sort: %w", err)
	}
	r, err := s.q.CreateTodo(ctx, sqlcgen.CreateTodoParams{
		WorkspaceID: workspaceID, Text: text, DueAt: dueAt, SortOrder: next,
	})
	if err != nil {
		return Todo{}, fmt.Errorf("create todo: %w", err)
	}
	return Todo{
		ID: r.ID, WorkspaceID: r.WorkspaceID,
		Text: r.Text, Done: r.Done == 1, DueAt: r.DueAt,
		SortOrder: r.SortOrder,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}, nil
}

// Update mutates the text/due/done in one shot.
func (s *TodosService) Update(ctx context.Context, id int64, text, dueAt string, done bool) error {
	d := int64(0)
	if done {
		d = 1
	}
	return s.q.UpdateTodo(ctx, sqlcgen.UpdateTodoParams{
		Text: text, DueAt: dueAt, Done: d, ID: id,
	})
}

// SetDone is the fast path for the row checkbox.
func (s *TodosService) SetDone(ctx context.Context, id int64, done bool) error {
	d := int64(0)
	if done {
		d = 1
	}
	return s.q.SetTodoDone(ctx, sqlcgen.SetTodoDoneParams{Done: d, ID: id})
}

// Delete removes a todo.
func (s *TodosService) Delete(ctx context.Context, id int64) error {
	return s.q.DeleteTodo(ctx, id)
}

// NotesService exposes per-workspace markdown notes.
type NotesService struct {
	q    *sqlcgen.Queries
	pool *sql.DB
}

// NewNotes wires the notes service to the shared pool.
func NewNotes(pool *sql.DB) *NotesService {
	return &NotesService{q: sqlcgen.New(pool), pool: pool}
}

// Note is the frontend-facing shape.
type Note struct {
	ID          int64  `json:"id"`
	WorkspaceID int64  `json:"workspaceId"`
	Title       string `json:"title"`
	BodyMD      string `json:"bodyMd"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// List returns notes for a workspace, most recently updated first.
func (s *NotesService) List(ctx context.Context, workspaceID int64) ([]Note, error) {
	rows, err := s.q.ListNotes(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	out := make([]Note, 0, len(rows))
	for _, r := range rows {
		out = append(out, Note{
			ID: r.ID, WorkspaceID: r.WorkspaceID,
			Title: r.Title, BodyMD: r.BodyMd,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}
	return out, nil
}

// Get returns one note by id.
func (s *NotesService) Get(ctx context.Context, id int64) (Note, error) {
	r, err := s.q.GetNote(ctx, id)
	if err != nil {
		return Note{}, fmt.Errorf("get note: %w", err)
	}
	return Note{
		ID: r.ID, WorkspaceID: r.WorkspaceID,
		Title: r.Title, BodyMD: r.BodyMd,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}, nil
}

// Create inserts a note. The body is markdown.
func (s *NotesService) Create(ctx context.Context, workspaceID int64, title, bodyMD string) (Note, error) {
	r, err := s.q.CreateNote(ctx, sqlcgen.CreateNoteParams{
		WorkspaceID: workspaceID, Title: title, BodyMd: bodyMD,
	})
	if err != nil {
		return Note{}, fmt.Errorf("create note: %w", err)
	}
	return Note{
		ID: r.ID, WorkspaceID: r.WorkspaceID,
		Title: r.Title, BodyMD: r.BodyMd,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}, nil
}

// Update mutates an existing note.
func (s *NotesService) Update(ctx context.Context, id int64, title, bodyMD string) error {
	return s.q.UpdateNote(ctx, sqlcgen.UpdateNoteParams{
		Title: title, BodyMd: bodyMD, ID: id,
	})
}

// Delete removes a note.
func (s *NotesService) Delete(ctx context.Context, id int64) error {
	return s.q.DeleteNote(ctx, id)
}

// Search returns notes in a workspace whose title or body contains q
// (case-insensitive substring match). LIKE is sufficient here — notes are
// short-lived and small; FTS would be overkill for a few hundred rows.
func (s *NotesService) Search(ctx context.Context, workspaceID int64, q string) ([]Note, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return s.List(ctx, workspaceID)
	}
	const query = `
		SELECT id, workspace_id, title, body_md, created_at, updated_at
		FROM notes
		WHERE workspace_id = ?
		  AND (LOWER(title) LIKE ? OR LOWER(body_md) LIKE ?)
		ORDER BY updated_at DESC
		LIMIT 100`
	like := "%" + strings.ToLower(q) + "%"
	rows, err := s.pool.QueryContext(ctx, query, workspaceID, like, like)
	if err != nil {
		return nil, fmt.Errorf("search notes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []Note{}
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.WorkspaceID, &n.Title, &n.BodyMD, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan note: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
