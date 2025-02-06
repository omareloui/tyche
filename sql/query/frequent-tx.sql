-- name: CreateFrequentTx :exec
INSERT INTO frequent_txs (
    id,
    frequency_by,
    frequency,
    account_id,
    amount,
    kind_id,
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
    sqlc.arg(frequency_by),
    sqlc.arg(frequency),
    sqlc.arg(account_id),
    ROUND(sqlc.arg(amount), 2),
    sqlc.arg(kind_id),
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

-- name: CreateFrequentTxTransaction :exec
INSERT INTO frequent_txs_transactions (
    transaction_id,
    frequency_id
)
VALUES (
    sqlc.arg(transaction_id),
    sqlc.arg(frequency_id)
);

-- name: GetFrequentTxs :many
SELECT
    sqlc.embed(ft),
    sqlc.embed(a),
    sqlc.embed(c),
    sqlc.embed(cp),
    sqlc.embed(ftta),
    sqlc.embed(fttc),
    sqlc.embed(k),
    sqlc.embed(cat),
    sqlc.embed(t),
    sqlc.embed(ta),
    sqlc.embed(tc),
    sqlc.embed(tcp),
    sqlc.embed(tta),
    sqlc.embed(ttc),
    sqlc.embed(tk),
    sqlc.embed(tcat),
    sqlc.embed(g),
    sqlc.embed(gc)
FROM frequent_txs AS ft
LEFT JOIN frequent_txs_transactions AS ftt ON ftt.frequency_id = ft.id
LEFT JOIN counterparties AS cp ON ft.counterparty_id = cp.id
LEFT JOIN accounts AS a ON ft.account_id = a.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN accounts AS ftta ON ft.to_account_id = a.id
LEFT JOIN currencies AS fttc ON ftta.currency_id = c.id
LEFT JOIN transaction_kinds AS k ON ft.kind_id = k.id
LEFT JOIN categories AS cat ON ft.category_id = cat.id
LEFT JOIN transactions AS t ON t.id = ftt.transaction_id
LEFT JOIN accounts AS ta ON t.account_id = ta.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN accounts AS tta ON t.to_account_id = tta.id
LEFT JOIN currencies AS ttc ON tta.currency_id = ttc.id
LEFT JOIN transaction_kinds AS tk ON t.kind_id = k.id
LEFT JOIN categories AS tcat ON t.category_id = tcat.id
LEFT JOIN goals AS g ON g.id = ft.goal_id
LEFT JOIN currencies AS gc ON gc.id = g.currency_id
LEFT JOIN counterparties AS tcp ON t.counterparty_id = tcp.id;


-- name: GetFrequentTx :many
SELECT
    sqlc.embed(ft),
    sqlc.embed(a),
    sqlc.embed(c),
    sqlc.embed(cp),
    sqlc.embed(ftta),
    sqlc.embed(fttc),
    sqlc.embed(k),
    sqlc.embed(cat),
    sqlc.embed(t),
    sqlc.embed(ta),
    sqlc.embed(tc),
    sqlc.embed(tcp),
    sqlc.embed(tta),
    sqlc.embed(ttc),
    sqlc.embed(tk),
    sqlc.embed(tcat),
    sqlc.embed(g),
    sqlc.embed(gc)
FROM frequent_txs AS ft
LEFT JOIN frequent_txs_transactions AS ftt ON ftt.frequency_id = ft.id
LEFT JOIN counterparties AS cp ON ft.counterparty_id = cp.id
LEFT JOIN accounts AS a ON ft.account_id = a.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN accounts AS ftta ON ft.to_account_id = a.id
LEFT JOIN currencies AS fttc ON ftta.currency_id = c.id
LEFT JOIN transaction_kinds AS k ON ft.kind_id = k.id
LEFT JOIN categories AS cat ON ft.category_id = cat.id
LEFT JOIN transactions AS t ON t.id = ftt.transaction_id
LEFT JOIN accounts AS ta ON t.account_id = ta.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN accounts AS tta ON t.to_account_id = tta.id
LEFT JOIN currencies AS ttc ON tta.currency_id = ttc.id
LEFT JOIN transaction_kinds AS tk ON t.kind_id = k.id
LEFT JOIN categories AS tcat ON t.category_id = tcat.id
LEFT JOIN counterparties AS tcp ON t.counterparty_id = tcp.id
LEFT JOIN goals AS g ON g.id = ft.goal_id
LEFT JOIN currencies AS gc ON gc.id = g.currency_id
WHERE ft.id = sqlc.arg(id);

-- name: UpdateFrequentTx :exec
UPDATE frequent_txs
SET
    frequency_by = sqlc.arg(frequency_by),
    frequency = sqlc.arg(frequency),
    account_id = sqlc.arg(account_id),
    amount = ROUND(sqlc.arg(amount), 2),
    kind_id = sqlc.arg(kind_id),
    category_id = sqlc.arg(category_id),
    counterparty_id = sqlc.arg(counterparty_id),
    description = sqlc.arg(description),
    to_account_id = sqlc.arg(to_account_id),
    to_amount = IIF(sqlc.narg(to_amount) > 0.0, ROUND(sqlc.narg(to_amount), 2), NULL),
    goal_id = sqlc.arg(goal_id),
    stops_at = sqlc.arg(stops_at),
    last_transaction_scheduled_to = sqlc.arg(last_transaction_scheduled_to)
WHERE id = sqlc.arg(id);


-- name: UpdateFrequentTxLastTransactionCreatedAt :exec
UPDATE frequent_txs
SET
    last_transaction_scheduled_to = sqlc.arg(last_transaction_scheduled_to)
WHERE id = sqlc.arg(id);


-- name: DeleteFrequentTx :exec
DELETE FROM frequent_txs
WHERE id = sqlc.arg(id);

-- name: DeleteScheduledTransactionsOfFrequentTx :exec
DELETE FROM transactions
WHERE rowid IN (
    SELECT tx.rowid FROM transactions AS tx
    LEFT JOIN frequent_txs_transactions AS ftxt ON tx.id = ftxt.transaction_id
    WHERE
        ftxt.frequency_id = sqlc.arg(frequency_id)
        AND tx.fulfilled_at IS NULL
);
