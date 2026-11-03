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
    starts_at,
    stops_at,
    last_transaction_scheduled_to
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(every),
    sqlc.arg(unit),
    sqlc.arg(account_id),
    ROUND(sqlc.arg(amount), 6),
    sqlc.arg(kind),
    sqlc.arg(category_id),
    sqlc.arg(counterparty_id),
    sqlc.arg(description),
    sqlc.arg(to_account_id),
    IIF(sqlc.narg(to_amount) > 0.0, ROUND(sqlc.narg(to_amount), 6), NULL),
    sqlc.arg(starts_at),
    sqlc.arg(stops_at),
    sqlc.arg(last_transaction_scheduled_to)
);



-- name: GetRecurrentTxs :many
SELECT
    sqlc.embed(rtd),
    ak.enum AS account_kind_enum,
    tak.enum AS to_account_kind_enum,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    cat.color AS category_color
FROM recurrent_txs_details AS rtd
LEFT JOIN account_kinds AS ak ON rtd.account_kind_id = ak.id
LEFT JOIN account_kinds AS tak ON rtd.to_account_kind_id = tak.id
LEFT JOIN counterparties AS cp ON rtd.recurrent_tx_counterparty_id = cp.id
LEFT JOIN categories AS cat ON rtd.recurrent_tx_category_id = cat.id
ORDER BY rtd.recurrent_tx_created_at DESC;


-- name: GetRecurrentTx :one
SELECT
    sqlc.embed(rtd),
    ak.enum AS account_kind_enum,
    tak.enum AS to_account_kind_enum,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    cat.color AS category_color
FROM recurrent_txs_details AS rtd
LEFT JOIN account_kinds AS ak ON rtd.account_kind_id = ak.id
LEFT JOIN account_kinds AS tak ON rtd.to_account_kind_id = tak.id
LEFT JOIN counterparties AS cp ON rtd.recurrent_tx_counterparty_id = cp.id
LEFT JOIN categories AS cat ON rtd.recurrent_tx_category_id = cat.id
WHERE rtd.recurrent_tx_id = sqlc.arg(id);


-- name: UpdateRecurrentTx :exec
UPDATE recurrent_txs
SET
    every = sqlc.arg(every),
    unit = sqlc.arg(unit),
    account_id = sqlc.arg(account_id),
    amount = ROUND(sqlc.arg(amount), 6),
    kind = sqlc.arg(kind),
    category_id = sqlc.arg(category_id),
    counterparty_id = sqlc.arg(counterparty_id),
    description = sqlc.arg(description),
    to_account_id = sqlc.arg(to_account_id),
    to_amount = IIF(sqlc.narg(to_amount) > 0.0, ROUND(sqlc.narg(to_amount), 6), NULL),
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

-- name: GetRecurrentTxsOccurrences :many
-- Dates of the transactions generated from recurrent txs, scheduled and
-- fulfilled ones.
SELECT
    tx.recurrent_id,
    tx.scheduled_to,
    tx.fulfilled_at
FROM transactions AS tx
WHERE tx.recurrent_id IS NOT NULL
ORDER BY tx.scheduled_to;

-- name: GetRecurrentTxOccurrences :many
SELECT
    tx.recurrent_id,
    tx.scheduled_to,
    tx.fulfilled_at
FROM transactions AS tx
WHERE tx.recurrent_id = sqlc.arg(recurrent_id)
ORDER BY tx.scheduled_to;
