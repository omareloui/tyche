-- name: GetTransactionKinds :many
SELECT
    id,
    kind
FROM transaction_kinds;


-- name: CreateTransactionKind :exec
INSERT INTO transaction_kinds (id, kind, increase_balance)
VALUES (?, ?, ?);
