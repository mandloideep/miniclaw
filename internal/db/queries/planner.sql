-- Calendar blocks, todos, and notes — the "planner" surface area for each
-- workspace. All three are workspace-scoped; nothing here syncs externally
-- until the calendar promotion path lands.

-- name: ListCalendarBlocks :many
SELECT id, workspace_id, title, notes, start_at, end_at, kind,
       google_event_id, created_at, updated_at
FROM calendar_blocks
WHERE workspace_id = ? AND end_at >= ?
ORDER BY start_at ASC;

-- name: CreateCalendarBlock :one
INSERT INTO calendar_blocks (workspace_id, title, notes, start_at, end_at, kind)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING id, workspace_id, title, notes, start_at, end_at, kind,
          google_event_id, created_at, updated_at;

-- name: UpdateCalendarBlock :exec
UPDATE calendar_blocks
SET title = ?, notes = ?, start_at = ?, end_at = ?, kind = ?,
    updated_at = datetime('now')
WHERE id = ?;

-- name: DeleteCalendarBlock :exec
DELETE FROM calendar_blocks WHERE id = ?;

-- name: SetCalendarGoogleEvent :exec
UPDATE calendar_blocks SET google_event_id = ?, updated_at = datetime('now')
WHERE id = ?;

-- name: ListTodos :many
SELECT id, workspace_id, text, done, due_at, sort_order, created_at, updated_at
FROM todos
WHERE workspace_id = ?
ORDER BY done ASC, sort_order ASC, id ASC;

-- name: NextTodoSort :one
SELECT CAST(COALESCE(MAX(sort_order) + 1, 0) AS INTEGER) AS next_sort
FROM todos WHERE workspace_id = ?;

-- name: CreateTodo :one
INSERT INTO todos (workspace_id, text, due_at, sort_order)
VALUES (?, ?, ?, ?)
RETURNING id, workspace_id, text, done, due_at, sort_order, created_at, updated_at;

-- name: UpdateTodo :exec
UPDATE todos
SET text = ?, due_at = ?, done = ?, updated_at = datetime('now')
WHERE id = ?;

-- name: SetTodoDone :exec
UPDATE todos SET done = ?, updated_at = datetime('now') WHERE id = ?;

-- name: DeleteTodo :exec
DELETE FROM todos WHERE id = ?;

-- name: ListNotes :many
SELECT id, workspace_id, title, body_md, created_at, updated_at
FROM notes
WHERE workspace_id = ?
ORDER BY updated_at DESC;

-- name: GetNote :one
SELECT id, workspace_id, title, body_md, created_at, updated_at
FROM notes WHERE id = ?;

-- name: CreateNote :one
INSERT INTO notes (workspace_id, title, body_md)
VALUES (?, ?, ?)
RETURNING id, workspace_id, title, body_md, created_at, updated_at;

-- name: UpdateNote :exec
UPDATE notes SET title = ?, body_md = ?, updated_at = datetime('now')
WHERE id = ?;

-- name: DeleteNote :exec
DELETE FROM notes WHERE id = ?;
