-- +goose Up
-- +goose StatementBegin
-- When the one-off reconversion of the foreign transactions at their day's
-- rate plus their account's fee ran (repository/fx_reconvert.go), NULL
-- until it did.
ALTER TABLE user ADD COLUMN fx_reconverted_at TIMESTAMP;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE user DROP COLUMN fx_reconverted_at;
-- +goose StatementEnd
