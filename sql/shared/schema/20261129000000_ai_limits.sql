-- +goose Up
-- +goose StatementBegin
-- The Google Search queries a grounded call ran, billed apart from tokens.
ALTER TABLE receipt_scans ADD COLUMN search_queries INTEGER NOT NULL DEFAULT 0;

-- A tenant's calls a calendar month of a kind (receipt, sms, col),
-- overriding the kind's default (RECEIPT_SCANS_PER_MONTH, ...).
CREATE TABLE ai_limits (
    tenant_id VARCHAR NOT NULL,
    kind VARCHAR NOT NULL,
    monthly_limit INTEGER NOT NULL CHECK (monthly_limit >= 0),
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tenant_id, kind)
);
INSERT INTO ai_limits (tenant_id, kind, monthly_limit, updated_at)
SELECT tenant_id, 'receipt', monthly_limit, updated_at FROM receipt_limits;
DROP TABLE receipt_limits;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE TABLE receipt_limits (
    tenant_id VARCHAR NOT NULL PRIMARY KEY,
    monthly_limit INTEGER NOT NULL CHECK (monthly_limit >= 0),
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO receipt_limits (tenant_id, monthly_limit, updated_at)
SELECT tenant_id, monthly_limit, updated_at FROM ai_limits WHERE kind = 'receipt';
DROP TABLE ai_limits;
ALTER TABLE receipt_scans DROP COLUMN search_queries;
-- +goose StatementEnd
