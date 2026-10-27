-- +goose Up
-- +goose StatementBegin
ALTER TABLE counterparties ADD COLUMN aliases JSON NOT NULL DEFAULT '[]';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE counterparties DROP COLUMN aliases;
-- +goose StatementEnd
