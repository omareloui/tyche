-- +goose Up
-- +goose StatementBegin
-- The lines of a transaction (what was bought, then the charges: VAT,
-- service, tip, delivery, fees, discounts, rounding), or of a recurrent
-- transaction, copied to its occurrences. Exactly one owner.
CREATE TABLE transaction_items (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    transaction_id VARCHAR REFERENCES transactions (id) ON DELETE CASCADE,
    recurrent_id VARCHAR REFERENCES recurrent_txs (id) ON DELETE CASCADE,
    -- models.TxItemKind: 0 item, else a charge.
    kind INTEGER NOT NULL DEFAULT 0,
    name VARCHAR NOT NULL DEFAULT '',
    quantity FLOAT NOT NULL DEFAULT 1,
    unit_price FLOAT NOT NULL DEFAULT 0,
    -- Negative for discounts.
    total FLOAT NOT NULL DEFAULT 0,
    -- A charge's percentage when the receipt shows one, e.g. VAT 14.
    rate FLOAT,
    -- A charge already inside the prices ("VAT included"): not summed.
    included BOOLEAN NOT NULL DEFAULT 0,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((transaction_id IS NULL) != (recurrent_id IS NULL))
);
CREATE INDEX transaction_items_tx ON transaction_items (transaction_id, position);
CREATE INDEX transaction_items_rtx ON transaction_items (recurrent_id, position);

-- A receipt's image, written when scanned (transaction_id NULL until the
-- transaction is saved; unlinked ones are purged after a day).
CREATE TABLE receipts (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    transaction_id VARCHAR REFERENCES transactions (id) ON DELETE CASCADE,
    data BLOB NOT NULL,
    content_type VARCHAR NOT NULL,
    hash VARCHAR NOT NULL,
    merchant VARCHAR NOT NULL DEFAULT '',
    -- What the OCR read, as is.
    ocr_json VARCHAR NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX receipts_tx ON receipts (transaction_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE receipts;
DROP TABLE transaction_items;
-- +goose StatementEnd
