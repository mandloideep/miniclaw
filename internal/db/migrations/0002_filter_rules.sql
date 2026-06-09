-- +goose Up
-- +goose StatementBegin
CREATE TABLE filter_rules (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id  INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    kind        TEXT    NOT NULL CHECK (kind IN ('block', 'allow')),
    -- exact email or domain match. lowercase comparisons.
    pattern     TEXT    NOT NULL,
    reason      TEXT    NOT NULL DEFAULT '',
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(account_id, pattern, kind)
);

CREATE INDEX filter_rules_account_idx ON filter_rules(account_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS filter_rules;
-- +goose StatementEnd
