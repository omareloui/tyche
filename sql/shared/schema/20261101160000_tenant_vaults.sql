-- +goose Up
-- +goose StatementBegin
-- The tenants whose database is encrypted (SQLCipher). Its key (DEK) is only
-- stored wrapped, once per slot: the user's password, their recovery code, a
-- signed in browser (session) or a mobile device, each slot's key derived
-- from a secret only the user or their browser/device has.
CREATE TABLE tenant_vaults (
    tenant_id VARCHAR NOT NULL PRIMARY KEY,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE vault_slots (
    id VARCHAR NOT NULL PRIMARY KEY,
    tenant_id VARCHAR NOT NULL REFERENCES tenant_vaults (tenant_id) ON DELETE CASCADE,
    kind VARCHAR NOT NULL CHECK (kind IN ('password', 'recovery', 'session', 'device')),
    -- Argon2id params (JSON) of password and recovery slots, NULL for the
    -- random secrets of sessions and devices.
    kdf_params VARCHAR,
    salt BLOB NOT NULL,
    wrapped_key BLOB NOT NULL,
    device_id VARCHAR,
    user_agent VARCHAR NOT NULL DEFAULT '',
    -- Sessions only: sliding, renewed while used.
    expires_at DATETIME,
    last_used_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX vault_slots_tenant_kind ON vault_slots (tenant_id, kind);
CREATE UNIQUE INDEX vault_slots_device ON vault_slots (tenant_id, device_id) WHERE device_id IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE vault_slots;
DROP TABLE tenant_vaults;
-- +goose StatementEnd
