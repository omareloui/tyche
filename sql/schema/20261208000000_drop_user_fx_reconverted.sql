-- +goose Up
-- +goose StatementBegin
-- The one-off reconversion of the foreign transactions is done and gone.
ALTER TABLE user DROP COLUMN fx_reconverted_at;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE user ADD COLUMN fx_reconverted_at TIMESTAMP;
-- +goose StatementEnd
