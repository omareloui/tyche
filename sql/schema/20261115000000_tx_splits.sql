-- +goose Up
-- +goose StatementBegin
-- The people a transaction's bill is split with, the user always being one
-- of them (not stored).
CREATE TABLE tx_split_people (
    transaction_id VARCHAR NOT NULL REFERENCES transactions (id) ON DELETE CASCADE,
    counterparty_id VARCHAR NOT NULL REFERENCES counterparties (id) ON DELETE CASCADE,
    position INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (transaction_id, counterparty_id)
);
CREATE INDEX tx_split_people_cp ON tx_split_people (counterparty_id);

-- Who shares an item of a split bill, equally: counterparty_id NULL is the
-- user. An item without any is the user's alone.
CREATE TABLE transaction_item_shares (
    item_id VARCHAR NOT NULL REFERENCES transaction_items (id) ON DELETE CASCADE,
    counterparty_id VARCHAR REFERENCES counterparties (id) ON DELETE CASCADE
);
CREATE INDEX transaction_item_shares_item ON transaction_item_shares (item_id);
CREATE INDEX transaction_item_shares_cp ON transaction_item_shares (counterparty_id);

-- An item shared by everyone in the split (the user and all the people,
-- also the ones added later), e.g. a dish for the table.
ALTER TABLE transaction_items ADD COLUMN shared_by_all BOOLEAN NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE transaction_items DROP COLUMN shared_by_all;
DROP TABLE transaction_item_shares;
DROP TABLE tx_split_people;
-- +goose StatementEnd
