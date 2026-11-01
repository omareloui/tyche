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
