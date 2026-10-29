-- name: CreateDevice :exec
INSERT INTO devices (
    id,
    name,
    install_id,
    token_id
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(name),
    sqlc.narg(install_id),
    sqlc.arg(token_id)
);

-- name: GetDeviceIDByInstallID :one
SELECT id
FROM devices
WHERE install_id = sqlc.arg(install_id)
LIMIT 1;

-- name: RenewDevice :exec
UPDATE devices
SET
    name = sqlc.arg(name),
    token_id = sqlc.arg(token_id),
    revoked_at = NULL
WHERE id = sqlc.arg(id);

-- name: GetDevices :many
SELECT
    sqlc.embed(devices)
FROM devices
ORDER BY created_at DESC;

-- name: GetActiveDevice :one
SELECT
    sqlc.embed(devices)
FROM devices
WHERE
    id = sqlc.arg(id)
    AND COALESCE(token_id, '') = sqlc.arg(token_id)
    AND revoked_at IS NULL
LIMIT 1;

-- name: TouchDevice :exec
UPDATE devices
SET
    last_seen_at = DATETIME('now')
WHERE id = sqlc.arg(id);

-- name: RevokeDevice :exec
UPDATE devices
SET
    revoked_at = DATETIME('now')
WHERE id = sqlc.arg(id) AND revoked_at IS NULL;
