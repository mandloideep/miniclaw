-- +goose Up
-- +goose StatementBegin
-- SQLite can't ALTER CHECK in place. Recreate accounts with a widened
-- auth_kind CHECK that allows ms_oauth (Microsoft Graph).

CREATE TABLE accounts_new (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id        INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    display_name        TEXT    NOT NULL,
    email_address       TEXT    NOT NULL,
    auth_kind           TEXT    NOT NULL CHECK (auth_kind IN ('imap', 'gmail_oauth', 'ms_oauth')),
    imap_host           TEXT,
    imap_port           INTEGER,
    smtp_host           TEXT,
    smtp_port           INTEGER,
    secret_ref          TEXT    NOT NULL,
    fetch_since         TEXT,
    sync_cadence_secs   INTEGER NOT NULL DEFAULT 300,
    last_synced_at      TEXT,
    ollama_model        TEXT    NOT NULL DEFAULT '',
    -- new: comma-separated allowlist of IMAP folders to walk; empty means INBOX only.
    folder_allowlist    TEXT    NOT NULL DEFAULT '',
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(email_address)
);

INSERT INTO accounts_new (id, workspace_id, display_name, email_address, auth_kind,
    imap_host, imap_port, smtp_host, smtp_port, secret_ref,
    fetch_since, sync_cadence_secs, last_synced_at, ollama_model,
    created_at, updated_at)
SELECT id, workspace_id, display_name, email_address, auth_kind,
    imap_host, imap_port, smtp_host, smtp_port, secret_ref,
    fetch_since, sync_cadence_secs, last_synced_at, ollama_model,
    created_at, updated_at
FROM accounts;

DROP TABLE accounts;
ALTER TABLE accounts_new RENAME TO accounts;
CREATE INDEX accounts_workspace_idx ON accounts(workspace_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Down isn't reversible safely (would drop ms_oauth rows). Best-effort.
DROP INDEX IF EXISTS accounts_workspace_idx;
CREATE TABLE accounts_old (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id        INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    display_name        TEXT    NOT NULL,
    email_address       TEXT    NOT NULL,
    auth_kind           TEXT    NOT NULL CHECK (auth_kind IN ('imap', 'gmail_oauth')),
    imap_host           TEXT,
    imap_port           INTEGER,
    smtp_host           TEXT,
    smtp_port           INTEGER,
    secret_ref          TEXT    NOT NULL,
    fetch_since         TEXT,
    sync_cadence_secs   INTEGER NOT NULL DEFAULT 300,
    last_synced_at      TEXT,
    ollama_model        TEXT    NOT NULL DEFAULT '',
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(email_address)
);
INSERT INTO accounts_old SELECT id, workspace_id, display_name, email_address, auth_kind,
    imap_host, imap_port, smtp_host, smtp_port, secret_ref,
    fetch_since, sync_cadence_secs, last_synced_at, ollama_model,
    created_at, updated_at FROM accounts WHERE auth_kind IN ('imap','gmail_oauth');
DROP TABLE accounts;
ALTER TABLE accounts_old RENAME TO accounts;
CREATE INDEX accounts_workspace_idx ON accounts(workspace_id);
-- +goose StatementEnd
