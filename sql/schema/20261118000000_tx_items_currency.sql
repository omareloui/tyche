-- +goose Up
-- +goose StatementBegin
-- The currency the lines are in, e.g. a receipt paid abroad: NULL is their
-- owner's account currency (following it when the account changes). Every
-- line of an owner has the same; converted with the transaction's own rate
-- (its amount / the lines' total). A currencies id, without a foreign key so
-- it drops like the other columns (currencies are never deleted).
ALTER TABLE transaction_items ADD COLUMN currency_id VARCHAR;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE transaction_items DROP COLUMN currency_id;
-- +goose StatementEnd
