-- +goose Up
-- +goose StatementBegin
-- How many of an item's units (quantity) the share is, e.g. 2 of 3 burgers;
-- the shares of an item are split by them (1 each: equally).
ALTER TABLE transaction_item_shares ADD COLUMN units FLOAT NOT NULL DEFAULT 1;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE transaction_item_shares DROP COLUMN units;
-- +goose StatementEnd
