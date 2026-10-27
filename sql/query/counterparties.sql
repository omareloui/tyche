-- name: CreateCounterparty :exec
INSERT INTO counterparties (
    id,
    name,
    aliases
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(name),
    sqlc.arg(aliases)
);

-- name: GetCounterparties :many
SELECT
    sqlc.embed(counterparties)
FROM counterparties
ORDER BY created_at DESC;

-- name: GetCounterparty :one
SELECT
    sqlc.embed(counterparties)
FROM counterparties
WHERE id = sqlc.arg(id);

-- name: UpdateCounterparty :exec
UPDATE counterparties
SET
    name = sqlc.arg(name),
    aliases = sqlc.arg(aliases)
WHERE id = sqlc.arg(id);

-- name: DeleteCounterparty :exec
DELETE FROM counterparties
WHERE id = sqlc.arg(id);
