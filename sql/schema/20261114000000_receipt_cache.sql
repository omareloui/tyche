-- +goose Up
-- +goose StatementBegin
-- A scanned image's sha256 finds what was read off it before (the cache).
CREATE INDEX receipts_hash ON receipts (hash);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX receipts_hash;
-- +goose StatementEnd
