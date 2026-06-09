-- name: ListPutAsideByWorkspace :many
SELECT e.id, e.account_id, e.from_address, e.from_name, e.subject, e.received_at
FROM emails e
JOIN accounts a ON a.id = e.account_id
WHERE a.workspace_id = ? AND e.is_put_aside = 1
ORDER BY e.received_at DESC;

-- name: ListUnscreenedByAccount :many
-- Senders the screener hasn't decided on yet, with the email that prompted them.
SELECT s.id AS sender_id, s.address, s.screener_state, s.first_seen_at,
       e.id AS email_id, e.subject, e.received_at
FROM senders s
JOIN emails e ON e.account_id = s.account_id AND e.from_address = s.address
WHERE s.account_id = ? AND s.screener_state = 'unscreened'
GROUP BY s.id
ORDER BY s.first_seen_at DESC;

-- name: ApproveSender :exec
UPDATE senders SET screener_state = 'approved' WHERE id = ?;

-- name: BlockSender :exec
UPDATE senders SET screener_state = 'blocked' WHERE id = ?;

-- name: UpsertSender :exec
INSERT INTO senders (account_id, address) VALUES (?, ?)
ON CONFLICT(account_id, address) DO NOTHING;

-- name: GetSenderState :one
SELECT screener_state FROM senders WHERE account_id = ? AND address = ?;

-- name: ListFilterRules :many
SELECT id, account_id, kind, pattern, reason, created_at
FROM filter_rules WHERE account_id = ? ORDER BY id;

-- name: AddFilterRule :one
INSERT INTO filter_rules (account_id, kind, pattern, reason)
VALUES (?, ?, ?, ?)
RETURNING id, account_id, kind, pattern, reason, created_at;

-- name: DeleteFilterRule :exec
DELETE FROM filter_rules WHERE id = ?;
