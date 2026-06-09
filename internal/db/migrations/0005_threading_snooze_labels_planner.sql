-- +goose Up
-- +goose StatementBegin

-- Threading + snooze on emails. SQLite ALTER TABLE ADD COLUMN can't add
-- NOT NULL without a default, so all four columns are nullable / defaulted.
-- thread_id is derived at ingest as the first References token (or
-- In-Reply-To, or the email's own Message-ID if no parent).
ALTER TABLE emails ADD COLUMN in_reply_to TEXT NOT NULL DEFAULT '';
ALTER TABLE emails ADD COLUMN email_refs  TEXT NOT NULL DEFAULT '';
ALTER TABLE emails ADD COLUMN thread_id   TEXT NOT NULL DEFAULT '';
ALTER TABLE emails ADD COLUMN snoozed_until TEXT;

CREATE INDEX emails_thread_idx ON emails(thread_id, received_at DESC)
    WHERE thread_id != '';
CREATE INDEX emails_snoozed_idx ON emails(snoozed_until)
    WHERE snoozed_until IS NOT NULL;

-- Gmail history cursor: lives on the account so per-account incremental
-- sync can use users.history.list once the initial Sync has populated it.
ALTER TABLE accounts ADD COLUMN gmail_history_id TEXT NOT NULL DEFAULT '';

-- email_labels: arbitrary provider-side labels (Gmail's CATEGORY_*,
-- IMPORTANT, STARRED, user labels). We don't model categories here — the
-- emails.category column still owns the four primary buckets.
CREATE TABLE email_labels (
    email_id INTEGER NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
    label    TEXT    NOT NULL,
    PRIMARY KEY (email_id, label)
);
CREATE INDEX email_labels_label_idx ON email_labels(label, email_id);

-- Calendar blocks: local-first. google_event_id stays empty until the
-- user promotes the block; only promoted blocks 2-way-sync with Google.
CREATE TABLE calendar_blocks (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id    INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    title           TEXT    NOT NULL,
    notes           TEXT    NOT NULL DEFAULT '',
    -- both stored as RFC3339 UTC; UI converts to user-local
    start_at        TEXT    NOT NULL,
    end_at          TEXT    NOT NULL,
    -- 'block' (time-block) | 'meeting' (intend-to-promote) | 'focus' (do-not-disturb)
    kind            TEXT    NOT NULL DEFAULT 'block'
        CHECK (kind IN ('block','meeting','focus')),
    google_event_id TEXT    NOT NULL DEFAULT '',
    created_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT    NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX calendar_blocks_ws_idx ON calendar_blocks(workspace_id, start_at);

-- Todos: lightweight per-workspace. due_at is RFC3339 or empty.
CREATE TABLE todos (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    text         TEXT    NOT NULL,
    done         INTEGER NOT NULL DEFAULT 0,
    due_at       TEXT    NOT NULL DEFAULT '',
    sort_order   INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT    NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX todos_ws_idx ON todos(workspace_id, done, sort_order);

-- Notes: simple markdown body per workspace. No revision history yet.
CREATE TABLE notes (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    title        TEXT    NOT NULL DEFAULT '',
    body_md      TEXT    NOT NULL DEFAULT '',
    created_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT    NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX notes_ws_idx ON notes(workspace_id, updated_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS notes;
DROP TABLE IF EXISTS todos;
DROP TABLE IF EXISTS calendar_blocks;
DROP TABLE IF EXISTS email_labels;
-- column drops require the SQLite 3.35+ syntax; both column- and index-drops
-- are best-effort because tests roll back via Down only when intentional.
DROP INDEX IF EXISTS emails_snoozed_idx;
DROP INDEX IF EXISTS emails_thread_idx;
ALTER TABLE emails DROP COLUMN snoozed_until;
ALTER TABLE emails DROP COLUMN thread_id;
ALTER TABLE emails DROP COLUMN email_refs;
ALTER TABLE emails DROP COLUMN in_reply_to;
ALTER TABLE accounts DROP COLUMN gmail_history_id;
-- +goose StatementEnd
