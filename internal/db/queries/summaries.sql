-- name: ListUnsummarizedByAccount :many
SELECT e.id, e.from_address, e.from_name, e.subject, e.received_at, e.body_plain
FROM emails e
LEFT JOIN summaries s ON s.email_id = e.id
WHERE e.account_id = ?
  AND s.email_id IS NULL
ORDER BY e.received_at DESC
LIMIT ?;

-- name: UpsertSummary :exec
INSERT INTO summaries (email_id, summary, needs_attention, attention_reason, model)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(email_id) DO UPDATE SET
    summary          = excluded.summary,
    needs_attention  = excluded.needs_attention,
    attention_reason = excluded.attention_reason,
    model            = excluded.model,
    generated_at     = datetime('now');

-- name: GetSummaryForEmail :one
SELECT email_id, summary, needs_attention, attention_reason, model, generated_at
FROM summaries
WHERE email_id = ?;

-- name: GetAccountModel :one
SELECT a.ollama_model AS account_model,
       s.default_model AS default_model
FROM accounts a, app_settings s
WHERE a.id = ? AND s.id = 1;
