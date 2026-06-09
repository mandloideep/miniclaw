-- +goose Up
-- +goose StatementBegin

CREATE TABLE workspaces (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL UNIQUE,
    emoji       TEXT    NOT NULL DEFAULT '',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE accounts (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id        INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    display_name        TEXT    NOT NULL,
    email_address       TEXT    NOT NULL,
    auth_kind           TEXT    NOT NULL CHECK (auth_kind IN ('imap', 'gmail_oauth')),
    -- IMAP/SMTP fields (NULL for oauth accounts)
    imap_host           TEXT,
    imap_port           INTEGER,
    smtp_host           TEXT,
    smtp_port           INTEGER,
    -- keychain lookup key for the secret (password or oauth refresh token)
    secret_ref          TEXT    NOT NULL,
    -- per-account ingest controls
    fetch_since         TEXT,   -- ISO date; NULL means "all"
    sync_cadence_secs   INTEGER NOT NULL DEFAULT 300,
    last_synced_at      TEXT,
    -- AI controls
    ollama_model        TEXT    NOT NULL DEFAULT '',
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(email_address)
);

CREATE INDEX accounts_workspace_idx ON accounts(workspace_id);

CREATE TABLE emails (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    message_id      TEXT    NOT NULL, -- RFC822 Message-ID, dedupe key
    folder          TEXT    NOT NULL DEFAULT 'INBOX',
    uid             INTEGER,          -- IMAP UID inside the folder
    from_address    TEXT    NOT NULL,
    from_name       TEXT    NOT NULL DEFAULT '',
    to_addresses    TEXT    NOT NULL DEFAULT '', -- comma-joined
    cc_addresses    TEXT    NOT NULL DEFAULT '',
    subject         TEXT    NOT NULL DEFAULT '',
    received_at     TEXT    NOT NULL,
    body_plain      TEXT    NOT NULL DEFAULT '',
    body_html       TEXT    NOT NULL DEFAULT '',
    headers_json    TEXT    NOT NULL DEFAULT '{}', -- raw header map
    is_read         INTEGER NOT NULL DEFAULT 0,
    is_put_aside    INTEGER NOT NULL DEFAULT 0,
    category        TEXT    NOT NULL DEFAULT '',   -- promotions/updates/social/newsletter/''
    created_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(account_id, message_id)
);

CREATE INDEX emails_account_received_idx ON emails(account_id, received_at DESC);
CREATE INDEX emails_unread_idx ON emails(account_id, is_read, received_at DESC);

-- FTS5 over subject + body_plain for in-app search
CREATE VIRTUAL TABLE emails_fts USING fts5(
    subject, body_plain,
    content='emails', content_rowid='id', tokenize='porter unicode61'
);

-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER emails_ai AFTER INSERT ON emails BEGIN
    INSERT INTO emails_fts(rowid, subject, body_plain)
    VALUES (new.id, new.subject, new.body_plain);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER emails_ad AFTER DELETE ON emails BEGIN
    INSERT INTO emails_fts(emails_fts, rowid, subject, body_plain)
    VALUES ('delete', old.id, old.subject, old.body_plain);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER emails_au AFTER UPDATE ON emails BEGIN
    INSERT INTO emails_fts(emails_fts, rowid, subject, body_plain)
    VALUES ('delete', old.id, old.subject, old.body_plain);
    INSERT INTO emails_fts(rowid, subject, body_plain)
    VALUES (new.id, new.subject, new.body_plain);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE summaries (
    email_id            INTEGER PRIMARY KEY REFERENCES emails(id) ON DELETE CASCADE,
    summary             TEXT    NOT NULL,
    needs_attention     INTEGER NOT NULL DEFAULT 0,
    attention_reason    TEXT    NOT NULL DEFAULT '',
    model               TEXT    NOT NULL,
    generated_at        TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX summaries_attention_idx ON summaries(needs_attention, generated_at DESC);

CREATE TABLE senders (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    address         TEXT    NOT NULL,
    -- screener: unscreened / approved / blocked
    screener_state  TEXT    NOT NULL DEFAULT 'unscreened'
        CHECK (screener_state IN ('unscreened','approved','blocked')),
    first_seen_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(account_id, address)
);

CREATE TABLE telegram_settings (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    bot_token   TEXT    NOT NULL DEFAULT '',
    digest_time TEXT    NOT NULL DEFAULT '08:00'
);

INSERT INTO telegram_settings (id) VALUES (1);

CREATE TABLE telegram_recipients (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,
    chat_id     TEXT    NOT NULL UNIQUE,
    created_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE workspace_recipients (
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    recipient_id INTEGER NOT NULL REFERENCES telegram_recipients(id) ON DELETE CASCADE,
    PRIMARY KEY (workspace_id, recipient_id)
);

CREATE TABLE account_recipients (
    account_id   INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    recipient_id INTEGER NOT NULL REFERENCES telegram_recipients(id) ON DELETE CASCADE,
    PRIMARY KEY (account_id, recipient_id)
);

CREATE TABLE app_settings (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    ollama_url      TEXT    NOT NULL DEFAULT 'http://localhost:11434',
    default_model   TEXT    NOT NULL DEFAULT ''
);

INSERT INTO app_settings (id) VALUES (1);

-- Seed default workspaces (idempotent via UNIQUE name)
INSERT INTO workspaces (name, emoji, sort_order) VALUES
    ('Family',   '👨‍👩‍👧', 0),
    ('Work',     '💼',     1),
    ('Personal', '🏠',     2),
    ('Other',    '📦',     3);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS account_recipients;
DROP TABLE IF EXISTS workspace_recipients;
DROP TABLE IF EXISTS telegram_recipients;
DROP TABLE IF EXISTS telegram_settings;
DROP TABLE IF EXISTS app_settings;
DROP TABLE IF EXISTS senders;
DROP TABLE IF EXISTS summaries;
DROP TRIGGER IF EXISTS emails_au;
DROP TRIGGER IF EXISTS emails_ad;
DROP TRIGGER IF EXISTS emails_ai;
DROP TABLE IF EXISTS emails_fts;
DROP TABLE IF EXISTS emails;
DROP TABLE IF EXISTS accounts;
DROP TABLE IF EXISTS workspaces;
-- +goose StatementEnd
