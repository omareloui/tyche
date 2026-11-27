-- +goose Up
-- +goose StatementBegin
-- ai_quick_add turns reading quick add's free text with AI on.
ALTER TABLE user ADD COLUMN ai_quick_add BOOLEAN NOT NULL DEFAULT FALSE;

-- quick_adds are the texts transactions were quick added from ("coffee 85
-- cib yesterday"), model the AI model that read it ('' when read without
-- one).
CREATE TABLE quick_adds (
    id TEXT PRIMARY KEY,
    text TEXT NOT NULL,
    model TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- quick_add_id is the text a transaction was quick added from.
ALTER TABLE transactions ADD COLUMN quick_add_id TEXT REFERENCES quick_adds (id) ON DELETE SET NULL;
CREATE INDEX transactions_quick_add ON transactions (quick_add_id) WHERE quick_add_id IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX transactions_quick_add;
ALTER TABLE transactions DROP COLUMN quick_add_id;
DROP TABLE quick_adds;
ALTER TABLE user DROP COLUMN ai_quick_add;
-- +goose StatementEnd
