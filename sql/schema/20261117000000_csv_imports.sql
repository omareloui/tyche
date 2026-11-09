-- +goose Up
-- +goose StatementBegin
-- How a bank's CSV export reads (models.CSVMapping JSON), saved per account
-- and header (header_sig, its normalized column names) to be picked again.
CREATE TABLE csv_mappings (
    id VARCHAR NOT NULL PRIMARY KEY,
    account_id VARCHAR NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    header_sig VARCHAR NOT NULL,
    mapping JSON NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (account_id, header_sig)
);
-- A CSV file imported into an account: pending (content kept for the
-- mapping and preview, purged after a day) until imported (content dropped).
CREATE TABLE csv_imports (
    id VARCHAR NOT NULL PRIMARY KEY,
    account_id VARCHAR NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    file_name VARCHAR NOT NULL DEFAULT '',
    content BLOB,
    mapping JSON NOT NULL DEFAULT '{}',
    status VARCHAR NOT NULL DEFAULT 'pending',
    row_count INTEGER NOT NULL DEFAULT 0,
    imported_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    imported_at TIMESTAMP
);
-- The import a transaction came from, and its row's hash (models.CSVRow.Hash)
-- telling a row imported before.
ALTER TABLE transactions ADD COLUMN import_id VARCHAR REFERENCES csv_imports (id) ON DELETE SET NULL;
ALTER TABLE transactions ADD COLUMN import_hash VARCHAR;
CREATE INDEX transactions_import_hash ON transactions (import_hash);
CREATE INDEX transactions_import_id ON transactions (import_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX transactions_import_id;
DROP INDEX transactions_import_hash;
ALTER TABLE transactions DROP COLUMN import_hash;
ALTER TABLE transactions DROP COLUMN import_id;
DROP TABLE csv_imports;
DROP TABLE csv_mappings;
-- +goose StatementEnd
