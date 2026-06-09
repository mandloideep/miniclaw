-- name: ListAccounts :many
SELECT id, workspace_id, display_name, email_address, auth_kind,
       imap_host, imap_port, smtp_host, smtp_port, secret_ref,
       fetch_since, sync_cadence_secs, last_synced_at, ollama_model,
       created_at, updated_at
FROM accounts
ORDER BY workspace_id, id;

-- name: ListAccountsByWorkspace :many
SELECT id, workspace_id, display_name, email_address, auth_kind,
       imap_host, imap_port, smtp_host, smtp_port, secret_ref,
       fetch_since, sync_cadence_secs, last_synced_at, ollama_model,
       created_at, updated_at
FROM accounts
WHERE workspace_id = ?
ORDER BY id;

-- name: GetAccount :one
SELECT id, workspace_id, display_name, email_address, auth_kind,
       imap_host, imap_port, smtp_host, smtp_port, secret_ref,
       fetch_since, sync_cadence_secs, last_synced_at, ollama_model,
       created_at, updated_at
FROM accounts
WHERE id = ?;

-- name: CreateIMAPAccount :one
INSERT INTO accounts (
    workspace_id, display_name, email_address, auth_kind,
    imap_host, imap_port, smtp_host, smtp_port,
    secret_ref, fetch_since, sync_cadence_secs, ollama_model
) VALUES (?, ?, ?, 'imap', ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, workspace_id, display_name, email_address, auth_kind,
          imap_host, imap_port, smtp_host, smtp_port, secret_ref,
          fetch_since, sync_cadence_secs, last_synced_at, ollama_model,
          created_at, updated_at;

-- name: CreateOAuthAccount :one
INSERT INTO accounts (
    workspace_id, display_name, email_address, auth_kind,
    secret_ref, fetch_since, sync_cadence_secs, ollama_model
) VALUES (?, ?, ?, 'gmail_oauth', ?, ?, ?, ?)
RETURNING id, workspace_id, display_name, email_address, auth_kind,
          imap_host, imap_port, smtp_host, smtp_port, secret_ref,
          fetch_since, sync_cadence_secs, last_synced_at, ollama_model,
          created_at, updated_at;

-- name: UpdateAccountSync :exec
UPDATE accounts
SET last_synced_at = datetime('now'), updated_at = datetime('now')
WHERE id = ?;

-- name: UpdateAccountCadence :exec
UPDATE accounts
SET sync_cadence_secs = ?, updated_at = datetime('now')
WHERE id = ?;

-- name: UpdateAccountFetchSince :exec
UPDATE accounts
SET fetch_since = ?, updated_at = datetime('now')
WHERE id = ?;

-- name: UpdateAccountModel :exec
UPDATE accounts
SET ollama_model = ?, updated_at = datetime('now')
WHERE id = ?;

-- name: DeleteAccount :exec
DELETE FROM accounts WHERE id = ?;
