-- +goose Up
-- +goose StatementBegin
-- Until when a purchase can be returned and is under warranty (user days,
-- midnight UTC like the days picked in forms), when it was returned, and a
-- merchant's usual return window (days) prefilling new purchases'.
ALTER TABLE transactions ADD COLUMN return_until DATE;
ALTER TABLE transactions ADD COLUMN warranty_until DATE;
ALTER TABLE transactions ADD COLUMN returned_at DATE;
ALTER TABLE counterparties ADD COLUMN return_days INTEGER CHECK (return_days BETWEEN 1 AND 365);
-- An item line's own warranty, else its transaction's applies.
ALTER TABLE transaction_items ADD COLUMN warranty_until DATE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE transaction_items DROP COLUMN warranty_until;
ALTER TABLE counterparties DROP COLUMN return_days;
ALTER TABLE transactions DROP COLUMN returned_at;
ALTER TABLE transactions DROP COLUMN warranty_until;
ALTER TABLE transactions DROP COLUMN return_until;
-- +goose StatementEnd
