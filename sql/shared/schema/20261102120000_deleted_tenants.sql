-- +goose Up
-- +goose StatementBegin
-- The tenants whose account an admin deleted. Their database isn't created
-- again from their old cookies or device tokens, only by signing in again.
CREATE TABLE deleted_tenants (
    tenant_id VARCHAR NOT NULL PRIMARY KEY,
    deleted_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) WITHOUT ROWID;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE deleted_tenants;
-- +goose StatementEnd
