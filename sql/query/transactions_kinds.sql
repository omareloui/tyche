-- name: GetTransactionKinds :many
SELECT
    id,
    kind,
    increase_balance
FROM transaction_kinds;

-- name: GetTransactionKind :one
SELECT
    id,
    kind,
    increase_balance
FROM transaction_kinds
WHERE id = sqlc.arg(id) LIMIT 1;

-- name: CreateTransactionKind :exec
INSERT INTO transaction_kinds (id, kind, increase_balance)
VALUES (
    sqlc.arg(id),
    sqlc.arg(kind),
    sqlc.arg(increase_balance)
);
