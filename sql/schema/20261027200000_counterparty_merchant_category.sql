-- +goose Up
-- +goose StatementBegin
-- Only merchants get their brand icon auto-detected.
ALTER TABLE counterparties ADD COLUMN is_merchant BOOLEAN NOT NULL DEFAULT FALSE;
UPDATE counterparties SET is_merchant = TRUE WHERE icon IS NOT NULL AND icon != '';
-- Used for new transactions with the counterparty and no category.
ALTER TABLE counterparties ADD COLUMN default_category_id VARCHAR REFERENCES categories (id) ON DELETE SET NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE counterparties DROP COLUMN default_category_id;
ALTER TABLE counterparties DROP COLUMN is_merchant;
-- +goose StatementEnd
