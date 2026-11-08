-- +goose Up
-- +goose StatementBegin
-- Every call reading a receipt, kept for the usage history and the monthly
-- limit. Costs are in millionths of a dollar, at the prices of the scan.
CREATE TABLE receipt_scans (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    tenant_id VARCHAR NOT NULL,
    scanned_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    model VARCHAR NOT NULL DEFAULT '',
    input_tokens INTEGER NOT NULL DEFAULT 0,
    -- Thinking tokens included, billed as output.
    output_tokens INTEGER NOT NULL DEFAULT 0,
    cost_micros INTEGER NOT NULL DEFAULT 0,
    ok BOOLEAN NOT NULL DEFAULT 1,
    error VARCHAR NOT NULL DEFAULT ''
);
CREATE INDEX receipt_scans_tenant ON receipt_scans (tenant_id, scanned_at);
CREATE INDEX receipt_scans_at ON receipt_scans (scanned_at);

-- A tenant's scans a calendar month, overriding RECEIPT_SCANS_PER_MONTH.
CREATE TABLE receipt_limits (
    tenant_id VARCHAR NOT NULL PRIMARY KEY,
    monthly_limit INTEGER NOT NULL CHECK (monthly_limit >= 0),
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE receipt_limits;
DROP TABLE receipt_scans;
-- +goose StatementEnd
