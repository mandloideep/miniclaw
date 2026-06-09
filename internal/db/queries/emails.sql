-- name: UpsertEmail :one
INSERT INTO emails (
    account_id, message_id, folder, uid,
    from_address, from_name, to_addresses, cc_addresses,
    subject, received_at, body_plain, body_html, headers_json,
    in_reply_to, email_refs, thread_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(account_id, message_id) DO UPDATE SET
    folder       = excluded.folder,
    uid          = excluded.uid,
    body_plain   = excluded.body_plain,
    body_html    = excluded.body_html,
    headers_json = excluded.headers_json,
    in_reply_to  = excluded.in_reply_to,
    email_refs   = excluded.email_refs,
    thread_id    = excluded.thread_id
RETURNING id;

-- name: ListEmailsByAccount :many
SELECT e.id, e.account_id, e.message_id, e.folder, e.uid,
       e.from_address, e.from_name, e.to_addresses, e.cc_addresses,
       e.subject, e.received_at, e.body_plain, e.is_read, e.is_put_aside, e.category,
       COALESCE(s.summary, '')          AS summary,
       COALESCE(s.needs_attention, 0)   AS needs_attention,
       COALESCE(s.attention_reason, '') AS attention_reason
FROM emails e
LEFT JOIN summaries s ON s.email_id = e.id
WHERE e.account_id = ?
  AND (e.snoozed_until IS NULL OR e.snoozed_until <= datetime('now'))
ORDER BY e.received_at DESC
LIMIT ?;

-- name: ListEmailsByWorkspace :many
SELECT e.id, e.account_id, e.message_id, e.folder, e.uid,
       e.from_address, e.from_name, e.to_addresses, e.cc_addresses,
       e.subject, e.received_at, e.body_plain, e.is_read, e.is_put_aside, e.category,
       COALESCE(s.summary, '')          AS summary,
       COALESCE(s.needs_attention, 0)   AS needs_attention,
       COALESCE(s.attention_reason, '') AS attention_reason
FROM emails e
JOIN accounts a ON a.id = e.account_id
LEFT JOIN summaries s ON s.email_id = e.id
WHERE a.workspace_id = ?
  AND (e.snoozed_until IS NULL OR e.snoozed_until <= datetime('now'))
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

-- name: SetSnooze :exec
UPDATE emails SET snoozed_until = ? WHERE id = ?;

-- name: ClearSnooze :exec
UPDATE emails SET snoozed_until = NULL WHERE id = ?;

-- name: ListSnoozedByWorkspace :many
SELECT e.id, e.account_id, e.from_address, e.from_name, e.subject,
       e.received_at, e.snoozed_until
FROM emails e
JOIN accounts a ON a.id = e.account_id
WHERE a.workspace_id = ? AND e.snoozed_until IS NOT NULL
ORDER BY e.snoozed_until ASC;

-- name: ListDueSnoozed :many
-- For the snooze ticker. Returns emails whose snoozed_until is <= now.
SELECT e.id, e.account_id, e.subject, e.snoozed_until,
       s.needs_attention, a.workspace_id
FROM emails e
JOIN accounts a ON a.id = e.account_id
LEFT JOIN summaries s ON s.email_id = e.id
WHERE e.snoozed_until IS NOT NULL AND e.snoozed_until <= ?;

-- name: ListEmailsByThread :many
SELECT id, account_id, from_address, from_name, subject, received_at,
       body_plain, is_read, is_put_aside, category
FROM emails
WHERE thread_id = ?
ORDER BY received_at ASC;

-- name: AddEmailLabel :exec
INSERT INTO email_labels (email_id, label) VALUES (?, ?)
ON CONFLICT(email_id, label) DO NOTHING;

-- name: ClearEmailLabels :exec
DELETE FROM email_labels WHERE email_id = ?;

-- name: ListEmailLabels :many
SELECT label FROM email_labels WHERE email_id = ? ORDER BY label;

-- name: ListNeedsAttentionSince :many
SELECT e.id, e.account_id, e.from_address, e.from_name, e.subject,
       e.received_at, s.summary, s.attention_reason, a.workspace_id
FROM emails e
JOIN summaries s ON s.email_id = e.id
JOIN accounts a ON a.id = e.account_id
WHERE s.needs_attention = 1
  AND s.generated_at >= ?
ORDER BY e.received_at DESC;
