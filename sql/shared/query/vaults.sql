-- name: ListVaultTenants :many
SELECT tenant_id FROM tenant_vaults;

-- name: CreateVault :exec
INSERT INTO tenant_vaults (tenant_id) VALUES (sqlc.arg(tenant_id));

-- name: DeleteVault :exec
DELETE FROM tenant_vaults WHERE tenant_id = sqlc.arg(tenant_id);

-- name: CreateVaultSlot :exec
INSERT INTO vault_slots (id, tenant_id, kind, kdf_params, salt, wrapped_key, device_id, user_agent, expires_at, last_used_at, created_at)
VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(kind), sqlc.narg(kdf_params), sqlc.arg(salt),
    sqlc.arg(wrapped_key), sqlc.narg(device_id), sqlc.arg(user_agent), sqlc.narg(expires_at), sqlc.narg(last_used_at), sqlc.arg(created_at)
);

-- name: GetVaultSlot :one
SELECT * FROM vault_slots WHERE id = sqlc.arg(id);

-- name: GetVaultSlotsByKind :many
SELECT * FROM vault_slots
WHERE tenant_id = sqlc.arg(tenant_id) AND kind = sqlc.arg(kind)
ORDER BY created_at DESC;

-- name: GetDeviceVaultSlot :one
SELECT * FROM vault_slots WHERE tenant_id = sqlc.arg(tenant_id) AND device_id = sqlc.arg(device_id);

-- name: DeleteVaultSlot :exec
DELETE FROM vault_slots WHERE id = sqlc.arg(id);

-- name: DeleteVaultSlotsByKind :exec
DELETE FROM vault_slots WHERE tenant_id = sqlc.arg(tenant_id) AND kind = sqlc.arg(kind);

-- name: DeleteOtherSessionSlots :exec
DELETE FROM vault_slots
WHERE tenant_id = sqlc.arg(tenant_id) AND kind = 'session' AND id != sqlc.arg(keep_id);

-- name: DeleteDeviceVaultSlot :exec
DELETE FROM vault_slots WHERE tenant_id = sqlc.arg(tenant_id) AND device_id = sqlc.arg(device_id);

-- name: DeleteDeviceVaultSlots :exec
DELETE FROM vault_slots WHERE tenant_id = sqlc.arg(tenant_id) AND kind = 'device';

-- name: TouchSessionSlot :exec
UPDATE vault_slots SET last_used_at = sqlc.arg(now), expires_at = sqlc.arg(expires_at)
WHERE id = sqlc.arg(id);

-- name: ListVaults :many
SELECT
    v.tenant_id,
    v.created_at,
    (SELECT COUNT(*) FROM vault_slots AS s WHERE s.tenant_id = v.tenant_id AND s.kind = 'session') AS sessions,
    (SELECT COUNT(*) FROM vault_slots AS s WHERE s.tenant_id = v.tenant_id AND s.kind = 'device') AS devices
FROM tenant_vaults AS v;

-- name: CreateVaultPasskey :exec
INSERT INTO vault_passkeys (slot_id, tenant_id, credential_id, credential, prf_salt, name)
VALUES (sqlc.arg(slot_id), sqlc.arg(tenant_id), sqlc.arg(credential_id), sqlc.arg(credential), sqlc.arg(prf_salt), sqlc.arg(name));

-- name: ListVaultPasskeys :many
SELECT p.slot_id, p.credential_id, p.credential, p.prf_salt, p.name, s.created_at, s.last_used_at
FROM vault_passkeys AS p
JOIN vault_slots AS s ON s.id = p.slot_id
WHERE p.tenant_id = sqlc.arg(tenant_id)
ORDER BY s.created_at DESC;

-- name: GetVaultPasskeySlot :one
SELECT s.*, p.credential, p.prf_salt
FROM vault_passkeys AS p
JOIN vault_slots AS s ON s.id = p.slot_id
WHERE p.tenant_id = sqlc.arg(tenant_id) AND p.credential_id = sqlc.arg(credential_id);

-- name: TouchVaultPasskey :exec
UPDATE vault_passkeys SET credential = sqlc.arg(credential) WHERE slot_id = sqlc.arg(slot_id);

-- name: TouchVaultSlot :exec
UPDATE vault_slots SET last_used_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- name: DeleteVaultPasskey :execrows
DELETE FROM vault_slots
WHERE id = sqlc.arg(slot_id) AND tenant_id = sqlc.arg(tenant_id) AND kind = 'passkey';
