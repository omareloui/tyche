-- +goose Up
-- +goose StatementBegin
-- is_archived hides the counterparty from the pickers and moves it to the
-- collapsed archived section of its page; its transactions keep it.
ALTER TABLE counterparties ADD COLUMN is_archived BOOL NOT NULL DEFAULT FALSE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE counterparties DROP COLUMN is_archived;
-- +goose StatementEnd
