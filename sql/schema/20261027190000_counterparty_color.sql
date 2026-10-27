-- +goose Up
-- +goose StatementBegin
-- The brand color of the icon, or a random one for the letter placeholder.
ALTER TABLE counterparties ADD COLUMN color VARCHAR;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE counterparties DROP COLUMN color;
-- +goose StatementEnd
