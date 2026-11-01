-- +goose Up
-- +goose StatementBegin
-- The latest rate of each base → quote pair (lowercase codes), shared by all
-- users and refreshed by the fx rates job.
CREATE TABLE fx_rates (
    base VARCHAR NOT NULL,
    quote VARCHAR NOT NULL,
    rate REAL NOT NULL,
    as_of DATE NOT NULL,
    fetched_at DATETIME NOT NULL,
    PRIMARY KEY (base, quote)
) WITHOUT ROWID;

-- Templates reading bank sms for every user, seeded from smsparser.Seeds.
CREATE TABLE sms_templates (
    id VARCHAR NOT NULL PRIMARY KEY,
    name VARCHAR NOT NULL,
    bank VARCHAR NOT NULL DEFAULT '',
    senders JSON NOT NULL DEFAULT '[]',
    action VARCHAR NOT NULL,
    tx_kind INTEGER,
    body VARCHAR NOT NULL,
    fixed JSON NOT NULL DEFAULT '{}',
    sample VARCHAR NOT NULL DEFAULT '',
    priority INTEGER NOT NULL DEFAULT 0,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    seed_key VARCHAR UNIQUE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE meta (
    key VARCHAR NOT NULL PRIMARY KEY,
    value VARCHAR NOT NULL
) WITHOUT ROWID;

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
DROP TABLE meta;
DROP TABLE sms_templates;
DROP TABLE fx_rates;
-- +goose StatementEnd
