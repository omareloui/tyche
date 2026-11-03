-- +goose Up
-- +goose StatementBegin
-- labels group counterparties, e.g. ["Restaurant", "Online store"], to filter
-- them by.
ALTER TABLE counterparties ADD COLUMN labels JSON NOT NULL DEFAULT '[]';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE counterparties DROP COLUMN labels;
-- +goose StatementEnd
