-- name: GetTransactionKinds :many
SELECT
    id,
    enum,
    increase_balance
FROM transaction_kinds
ORDER BY enum;

-- name: GetTransactionKind :one
SELECT
    id,
    enum,
    increase_balance
FROM transaction_kinds
WHERE id = sqlc.arg(id) LIMIT 1;

-- name: CreateTransactionKind :exec
INSERT INTO transaction_kinds (id, enum, increase_balance)
VALUES (
    sqlc.arg(id),
    sqlc.arg(enum),
    sqlc.arg(increase_balance)
);
