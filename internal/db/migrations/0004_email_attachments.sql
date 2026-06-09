-- +goose Up
-- +goose StatementBegin
-- email_attachments holds inline image parts and (eventually) file
-- attachments. We persist bytes directly into SQLite instead of writing
-- a sidecar file because:
--   - everything's local-only, so disk-rename / GC stories don't apply,
--   - SQLite handles megabyte blobs without breaking a sweat,
--   - a single DELETE on emails cascades the rows away.
-- content_id is the RFC 2392 cid (without the angle brackets), used by
-- the frontend to rewrite <img src="cid:..."> into the asset URL we
-- serve.
CREATE TABLE email_attachments (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    email_id    INTEGER NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
    content_id  TEXT    NOT NULL DEFAULT '',  -- cid:<value>; empty for non-inline
    filename    TEXT    NOT NULL DEFAULT '',
    mime_type   TEXT    NOT NULL DEFAULT 'application/octet-stream',
    size_bytes  INTEGER NOT NULL DEFAULT 0,
    data        BLOB    NOT NULL,
    is_inline   INTEGER NOT NULL DEFAULT 0,   -- 1 if Content-Disposition: inline OR carries Content-ID
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(email_id, content_id, filename)
);
CREATE INDEX email_attachments_email_idx ON email_attachments(email_id);
CREATE INDEX email_attachments_cid_idx ON email_attachments(email_id, content_id)
    WHERE content_id != '';
-- +goose StatementEnd
