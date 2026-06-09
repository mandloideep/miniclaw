-- name: GetTelegramSettings :one
SELECT bot_token, digest_time FROM telegram_settings WHERE id = 1;

-- name: SetTelegramBotToken :exec
UPDATE telegram_settings SET bot_token = ? WHERE id = 1;

-- name: SetDigestTime :exec
UPDATE telegram_settings SET digest_time = ? WHERE id = 1;

-- name: ListRecipients :many
SELECT id, name, chat_id, created_at
FROM telegram_recipients
ORDER BY name;

-- name: CreateRecipient :one
INSERT INTO telegram_recipients (name, chat_id) VALUES (?, ?)
RETURNING id, name, chat_id, created_at;

-- name: DeleteRecipient :exec
DELETE FROM telegram_recipients WHERE id = ?;

-- name: AssignRecipientToWorkspace :exec
INSERT OR IGNORE INTO workspace_recipients (workspace_id, recipient_id) VALUES (?, ?);

-- name: UnassignRecipientFromWorkspace :exec
DELETE FROM workspace_recipients WHERE workspace_id = ? AND recipient_id = ?;

-- name: AssignRecipientToAccount :exec
INSERT OR IGNORE INTO account_recipients (account_id, recipient_id) VALUES (?, ?);

-- name: UnassignRecipientFromAccount :exec
DELETE FROM account_recipients WHERE account_id = ? AND recipient_id = ?;

-- name: ListRecipientsForWorkspace :many
SELECT r.id, r.name, r.chat_id
FROM telegram_recipients r
JOIN workspace_recipients wr ON wr.recipient_id = r.id
WHERE wr.workspace_id = ?;

-- name: ListRecipientsForAccount :many
SELECT r.id, r.name, r.chat_id
FROM telegram_recipients r
JOIN account_recipients ar ON ar.recipient_id = r.id
WHERE ar.account_id = ?;

-- Combined fan-out: union of workspace and account assignments for a
-- given account, so callers can fire a notify with one query.
-- name: ListRecipientsForFanout :many
SELECT DISTINCT r.id, r.name, r.chat_id
FROM telegram_recipients r
WHERE r.id IN (
    SELECT wr.recipient_id
    FROM workspace_recipients wr
    JOIN accounts a ON a.workspace_id = wr.workspace_id
    WHERE a.id = sqlc.arg('account_id')
    UNION
    SELECT ar.recipient_id
    FROM account_recipients ar
    WHERE ar.account_id = sqlc.arg('account_id')
);
