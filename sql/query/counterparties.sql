-- name: CreateCounterparty :exec
INSERT INTO counterparties (
    id,
    name,
    aliases,
    icon,
    color
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(name),
    sqlc.arg(aliases),
    sqlc.arg(icon),
    sqlc.arg(color)
);

-- name: GetCounterparties :many
SELECT
    sqlc.embed(counterparties)
FROM counterparties
ORDER BY name COLLATE NOCASE;

-- name: GetCounterparty :one
SELECT
    sqlc.embed(counterparties)
FROM counterparties
WHERE id = sqlc.arg(id);

-- name: UpdateCounterparty :exec
UPDATE counterparties
SET
    name = sqlc.arg(name),
    aliases = sqlc.arg(aliases),
    icon = sqlc.arg(icon),
    color = sqlc.arg(color)
WHERE id = sqlc.arg(id);

-- name: SetCounterpartyAppearance :exec
UPDATE counterparties
SET
    icon = sqlc.arg(icon),
    color = sqlc.arg(color)
WHERE id = sqlc.arg(id);

-- name: DeleteCounterparty :exec
DELETE FROM counterparties
WHERE id = sqlc.arg(id);
