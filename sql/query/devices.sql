-- name: CreateDevice :exec
INSERT INTO devices (
    id,
    name
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(name)
);

-- name: GetDevices :many
SELECT
    sqlc.embed(devices)
FROM devices
ORDER BY created_at DESC;

-- name: GetActiveDevice :one
SELECT
    sqlc.embed(devices)
FROM devices
WHERE id = sqlc.arg(id) AND revoked_at IS NULL
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
