-- +goose Up
-- +goose StatementBegin
-- A scan answered from the tenant's cache: no call, no tokens, not counted
-- toward the monthly limit.
ALTER TABLE receipt_scans ADD COLUMN cached BOOLEAN NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE receipt_scans DROP COLUMN cached;
-- +goose StatementEnd
