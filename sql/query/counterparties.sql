-- name: CreateCounterparty :exec
INSERT INTO counterparties (
    id,
    name
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(name)
);

-- name: GetCounterparties :many
SELECT
    id,
    name,
    created_at
FROM counterparties
ORDER BY created_at DESC;

-- name: GetCounterparty :one
SELECT
    id,
    name,
    created_at
FROM counterparties
WHERE id = sqlc.arg(id);

-- name: UpdateCounterparty :exec
UPDATE counterparties
SET
    name = sqlc.arg(name)
WHERE id = sqlc.arg(id);

-- name: DeleteCounterparty :exec
DELETE FROM counterparties
WHERE id = sqlc.arg(id);
