-- name: CreateCounterParty :exec
INSERT INTO counter_parties (
    id,
    name
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(name)
);

-- name: GetCounterParties :many
SELECT
    id,
    name,
    created_at
FROM counter_parties
ORDER BY created_at DESC;

-- name: GetCounterParty :one
SELECT
    id,
    name,
    created_at
FROM counter_parties
WHERE id = sqlc.arg(id);

-- name: UpdateCounterParty :exec
UPDATE counter_parties
SET
    name = sqlc.arg(name)
WHERE id = sqlc.arg(id);

-- name: DeleteCounterParty :exec
DELETE FROM counter_parties
WHERE id = sqlc.arg(id);
