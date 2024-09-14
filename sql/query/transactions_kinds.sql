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
WHERE id = ? LIMIT 1;

-- name: CreateTransactionKind :exec
INSERT INTO transaction_kinds (id, kind, increase_balance)
VALUES (?, ?, ?);
