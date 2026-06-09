-- name: ListWorkspaces :many
SELECT id, name, emoji, sort_order, created_at, updated_at
FROM workspaces
ORDER BY sort_order, id;

-- name: GetWorkspace :one
SELECT id, name, emoji, sort_order, created_at, updated_at
FROM workspaces
WHERE id = ?;

-- name: CreateWorkspace :one
INSERT INTO workspaces (name, emoji, sort_order)
VALUES (?, ?, COALESCE((SELECT MAX(sort_order) + 1 FROM workspaces), 0))
RETURNING id, name, emoji, sort_order, created_at, updated_at;

-- name: UpdateWorkspace :exec
UPDATE workspaces
SET name = ?, emoji = ?, updated_at = datetime('now')
WHERE id = ?;

-- name: DeleteWorkspace :exec
DELETE FROM workspaces WHERE id = ?;

-- name: ReorderWorkspace :exec
UPDATE workspaces SET sort_order = ?, updated_at = datetime('now') WHERE id = ?;
