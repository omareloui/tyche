-- name: CreateRecurrentTx :exec
INSERT INTO recurrent_txs (
    id,
    every,
    unit,
    account_id,
    amount,
    kind,
    category_id,
    counterparty_id,
    description,
    to_account_id,
    to_amount,
    goal_id,
    starts_at,
    stops_at,
    last_transaction_scheduled_to
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(every),
    sqlc.arg(unit),
    sqlc.arg(account_id),
    ROUND(sqlc.arg(amount), 2),
    sqlc.arg(kind),
    sqlc.arg(category_id),
    sqlc.arg(counterparty_id),
    sqlc.arg(description),
    sqlc.arg(to_account_id),
    IIF(sqlc.narg(to_amount) > 0.0, ROUND(sqlc.narg(to_amount), 2), NULL),
    sqlc.arg(goal_id),
    sqlc.arg(starts_at),
    sqlc.arg(stops_at),
    sqlc.arg(last_transaction_scheduled_to)
);



-- name: GetRecurrentTxs :many
SELECT * FROM recurrent_txs_details;


-- name: GetRecurrentTx :one
SELECT * FROM recurrent_txs_details
WHERE recurrent_tx_id = sqlc.arg(id);


-- name: UpdateRecurrentTx :exec
UPDATE recurrent_txs
SET
    every = sqlc.arg(every),
    unit = sqlc.arg(unit),
    account_id = sqlc.arg(account_id),
    amount = ROUND(sqlc.arg(amount), 2),
    kind = sqlc.arg(kind),
    category_id = sqlc.arg(category_id),
    counterparty_id = sqlc.arg(counterparty_id),
    description = sqlc.arg(description),
    to_account_id = sqlc.arg(to_account_id),
    to_amount = IIF(sqlc.narg(to_amount) > 0.0, ROUND(sqlc.narg(to_amount), 2), NULL),
    goal_id = sqlc.arg(goal_id),
    stops_at = sqlc.arg(stops_at),
    last_transaction_scheduled_to = sqlc.arg(last_transaction_scheduled_to)
WHERE id = sqlc.arg(id);


-- name: UpdateRecurrentTxLastTransactionCreatedAt :exec
UPDATE recurrent_txs
SET
    last_transaction_scheduled_to = sqlc.arg(last_transaction_scheduled_to)
WHERE id = sqlc.arg(id);


-- name: DeleteRecurrentTx :exec
DELETE FROM recurrent_txs
WHERE id = sqlc.arg(id);

-- name: DeleteScheduledTransactionsOfRecurrentTx :exec
DELETE FROM transactions AS tx
WHERE (
    tx.recurrent_id = sqlc.arg(recurrent_id)
    AND tx.fulfilled_at IS NULL
);
