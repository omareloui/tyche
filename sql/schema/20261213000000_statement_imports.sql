-- +goose Up
-- +goose StatementBegin
-- historical transactions are past data imported (a CSV or a PDF statement)
-- that never moved accounts.amount: the balance already holds them.
ALTER TABLE transactions ADD COLUMN historical BOOLEAN NOT NULL DEFAULT FALSE;
-- An import's source: a CSV file, or a PDF statement read by the AI into CSV
-- rows (content), source_hash its sha256 so the same PDF isn't read again,
-- warning what reading it found off (rows possibly missed, another account).
ALTER TABLE csv_imports ADD COLUMN source_kind VARCHAR NOT NULL DEFAULT 'csv';
ALTER TABLE csv_imports ADD COLUMN source_hash VARCHAR;
ALTER TABLE csv_imports ADD COLUMN warning TEXT NOT NULL DEFAULT '';
CREATE INDEX csv_imports_source_hash ON csv_imports (source_hash);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX csv_imports_source_hash;
ALTER TABLE csv_imports DROP COLUMN warning;
ALTER TABLE csv_imports DROP COLUMN source_hash;
ALTER TABLE csv_imports DROP COLUMN source_kind;
ALTER TABLE transactions DROP COLUMN historical;
-- +goose StatementEnd
