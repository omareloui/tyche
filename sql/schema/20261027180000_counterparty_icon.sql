-- +goose Up
-- +goose StatementBegin
-- NULL means the icon was never detected, '' means no icon.
ALTER TABLE counterparties ADD COLUMN icon VARCHAR;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE counterparties DROP COLUMN icon;
-- +goose StatementEnd
