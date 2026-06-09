-- name: UpsertEmail :one
INSERT INTO emails (
    account_id, message_id, folder, uid,
    from_address, from_name, to_addresses, cc_addresses,
    subject, received_at, body_plain, body_html, headers_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(account_id, message_id) DO UPDATE SET
    folder       = excluded.folder,
    uid          = excluded.uid,
    body_plain   = excluded.body_plain,
    body_html    = excluded.body_html,
    headers_json = excluded.headers_json
RETURNING id;

-- name: ListEmailsByAccount :many
SELECT id, account_id, message_id, folder, uid,
       from_address, from_name, to_addresses, cc_addresses,
       subject, received_at, body_plain, is_read, is_put_aside, category
FROM emails
WHERE account_id = ?
ORDER BY received_at DESC
LIMIT ?;

-- name: ListEmailsByWorkspace :many
SELECT e.id, e.account_id, e.message_id, e.folder, e.uid,
       e.from_address, e.from_name, e.to_addresses, e.cc_addresses,
       e.subject, e.received_at, e.body_plain, e.is_read, e.is_put_aside, e.category
FROM emails e
JOIN accounts a ON a.id = e.account_id
WHERE a.workspace_id = ?
ORDER BY e.received_at DESC
LIMIT ?;

-- name: MaxUIDForFolder :one
SELECT CAST(COALESCE(MAX(uid), 0) AS INTEGER) AS max_uid
FROM emails
WHERE account_id = ? AND folder = ?;

-- name: MarkEmailRead :exec
UPDATE emails SET is_read = 1 WHERE id = ?;

-- name: MarkEmailUnread :exec
UPDATE emails SET is_read = 0 WHERE id = ?;

-- name: TogglePutAside :exec
UPDATE emails SET is_put_aside = 1 - is_put_aside WHERE id = ?;

-- name: SetCategory :exec
UPDATE emails SET category = ? WHERE id = ?;

-- name: ListNeedsAttentionSince :many
SELECT e.id, e.account_id, e.from_address, e.from_name, e.subject,
       e.received_at, s.summary, s.attention_reason, a.workspace_id
FROM emails e
JOIN summaries s ON s.email_id = e.id
JOIN accounts a ON a.id = e.account_id
WHERE s.needs_attention = 1
  AND s.generated_at >= ?
ORDER BY e.received_at DESC;
