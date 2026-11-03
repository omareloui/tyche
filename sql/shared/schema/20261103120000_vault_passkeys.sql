-- +goose Up
-- +goose StatementBegin
-- Rebuilt to add the passkey kind to the check (nothing points at it).
CREATE TABLE vault_slots_new (
    id VARCHAR NOT NULL PRIMARY KEY,
    tenant_id VARCHAR NOT NULL REFERENCES tenant_vaults (tenant_id) ON DELETE CASCADE,
    kind VARCHAR NOT NULL CHECK (kind IN ('password', 'recovery', 'session', 'device', 'passkey')),
    -- Argon2id params (JSON) of password and recovery slots, NULL for the
    -- random secrets of sessions, devices and passkeys.
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

INSERT INTO vault_slots_new SELECT * FROM vault_slots;
DROP TABLE vault_slots;
ALTER TABLE vault_slots_new RENAME TO vault_slots;

CREATE INDEX vault_slots_tenant_kind ON vault_slots (tenant_id, kind);
CREATE UNIQUE INDEX vault_slots_device ON vault_slots (tenant_id, device_id) WHERE device_id IS NOT NULL;

-- The WebAuthn credential of a passkey slot, whose key is wrapped with the
-- credential's PRF output for prf_salt (never stored).
CREATE TABLE vault_passkeys (
    slot_id VARCHAR NOT NULL PRIMARY KEY REFERENCES vault_slots (id) ON DELETE CASCADE,
    tenant_id VARCHAR NOT NULL,
    credential_id BLOB NOT NULL UNIQUE,
    -- The go-webauthn credential (public key, sign count, flags…) as JSON.
    credential VARCHAR NOT NULL,
    prf_salt BLOB NOT NULL,
    name VARCHAR NOT NULL DEFAULT ''
);

CREATE INDEX vault_passkeys_tenant ON vault_passkeys (tenant_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE vault_passkeys;
DELETE FROM vault_slots WHERE kind = 'passkey';

CREATE TABLE vault_slots_old (
    id VARCHAR NOT NULL PRIMARY KEY,
    tenant_id VARCHAR NOT NULL REFERENCES tenant_vaults (tenant_id) ON DELETE CASCADE,
    kind VARCHAR NOT NULL CHECK (kind IN ('password', 'recovery', 'session', 'device')),
    kdf_params VARCHAR,
    salt BLOB NOT NULL,
    wrapped_key BLOB NOT NULL,
    device_id VARCHAR,
    user_agent VARCHAR NOT NULL DEFAULT '',
    expires_at DATETIME,
    last_used_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO vault_slots_old SELECT * FROM vault_slots;
DROP TABLE vault_slots;
ALTER TABLE vault_slots_old RENAME TO vault_slots;

CREATE INDEX vault_slots_tenant_kind ON vault_slots (tenant_id, kind);
CREATE UNIQUE INDEX vault_slots_device ON vault_slots (tenant_id, device_id) WHERE device_id IS NOT NULL;
-- +goose StatementEnd
