-- +goose Up
-- +goose StatementBegin
-- The catalog of what was bought: a receipt line (transaction_items) is
-- linked to one, so the same product is followed across transactions and
-- merchants.
CREATE TABLE items (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR NOT NULL,
    -- models.ItemKey(name): what lines are matched by.
    name_key VARCHAR NOT NULL,
    -- JSON list of other names (receipt spellings), like counterparties.aliases.
    aliases VARCHAR NOT NULL DEFAULT '[]',
    category_id VARCHAR REFERENCES categories (id) ON DELETE SET NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX items_name_key ON items (name_key);

-- The merchants an item was bought at (kept on every save, never pruned).
CREATE TABLE counterparty_items (
    counterparty_id VARCHAR NOT NULL REFERENCES counterparties (id) ON DELETE CASCADE,
    item_id VARCHAR NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    PRIMARY KEY (counterparty_id, item_id)
);
CREATE INDEX counterparty_items_item ON counterparty_items (item_id);

-- transaction_items is the transaction <-> item join (quantity, unit price,
-- total and currency stay on it; name keeps the line's own spelling). NULL
-- for charges and the lines not linked yet.
ALTER TABLE transaction_items ADD COLUMN item_id VARCHAR REFERENCES items (id) ON DELETE SET NULL;
CREATE INDEX transaction_items_item ON transaction_items (item_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX transaction_items_item;
ALTER TABLE transaction_items DROP COLUMN item_id;
DROP TABLE counterparty_items;
DROP TABLE items;
-- +goose StatementEnd
