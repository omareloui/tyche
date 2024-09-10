-- name: GetTransactionKinds :many
SELECT
    id,
    kind,
    increase_balance
FROM transaction_kinds;

-- name: CreateTransactionKind :exec
INSERT INTO transaction_kinds (id, kind, increase_balance)
VALUES (?, ?, ?);
