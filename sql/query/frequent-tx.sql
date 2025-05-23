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

-- name: GetFrequentTxsWTxs :many
SELECT
    sqlc.embed(ft),
    sqlc.embed(a),
    sqlc.embed(c),
    sqlc.embed(k),
    cp.name AS counterparty_name,
    ftta.id AS frequent_to_account_id,
    ftta.name AS frequent_to_account_name,
    ftta.kind_id AS frequent_to_account_kind_id,
    ftta.amount AS frequent_to_account_amount,
    ftta.currency_id AS frequent_to_account_currency_id,
    ftta.counterparty_id AS frequent_to_account_counterparty_id,
    ftta.card_number AS frequent_to_account_card_number,
    ftta.card_limit AS frequent_to_account_card_limit,
    ftta.name_on_card AS frequent_to_account_name_on_card,
    ftta.iban AS frequent_to_account_iban,
    ftta.note AS frequent_to_account_note,
    ftta.is_archived AS frequent_to_account_is_archived,
    ftta.exclude_from_balance AS frequent_to_account_exclude_from_balance,
    ftta.expiration_date AS frequent_to_account_expiration_date,
    ftta.created_at AS frequent_to_account_created_at,
    fttc.id AS frequent_to_account_currency_id,
    fttc.string AS frequent_to_account_currency_string,
    fttc.symbol AS frequent_to_account_currency_symbol,
    cat.id AS category_id,
    cat.name AS category_name,
    cat.budget_amount AS category_budget_amount,
    cat.budget_currency_id AS category_budget_currency_id,
    cat.kind_id AS category_kind_id,
    cat.created_at AS category_created_at,
    g.name AS goal_name,
    g.price AS goal_price,
    g.currency_id AS goal_currency_id,
    g.fulfilled_at AS goal_fulfilled_at,
    gc.string AS goal_currency_string,
    gc.symbol AS goal_currency_symbol,
    t.id AS transaction_id,
    t.account_id AS transaction_account_id,
    t.amount AS transaction_amount,
    t.kind_id AS transaction_kind_id,
    t.category_id AS transaction_category_id,
    t.counterparty_id AS transaction_counterparty_id,
    t.description AS transaction_description,
    t.to_account_id AS transaction_to_account_id,
    t.to_amount AS transaction_to_amount,
    t.goal_id AS transaction_goal_id,
    t.fulfilled_at AS transaction_fulfilled_at,
    t.scheduled_to AS transaction_scheduled_to,
    t.created_at AS transaction_created_at,
    ta.id AS transaction_account_id,
    ta.name AS transaction_account_name,
    ta.kind_id AS transaction_account_kind_id,
    ta.amount AS transaction_account_amount,
    ta.currency_id AS transaction_account_currency_id,
    ta.counterparty_id AS transaction_account_counterparty_id,
    ta.card_number AS transaction_account_card_number,
    ta.card_limit AS transaction_account_card_limit,
    ta.name_on_card AS transaction_account_name_on_card,
    ta.iban AS transaction_account_iban,
    ta.note AS transaction_account_note,
    ta.is_archived AS transaction_account_is_archived,
    ta.exclude_from_balance AS transaction_account_exclude_from_balance,
    ta.expiration_date AS transaction_account_expiration_date,
    ta.created_at AS transaction_account_created_at,
    tc.string AS transaction_currency_string,
    tc.symbol AS transaction_currency_symbol,
    tcp.name AS transaction_counterparty_name,
    tta.id AS transaction_to_account_id,
    tta.name AS transaction_to_account_name,
    tta.kind_id AS transaction_to_account_kind_id,
    tta.amount AS transaction_to_account_amount,
    tta.currency_id AS transaction_to_account_currency_id,
    tta.counterparty_id AS transaction_to_account_counterparty_id,
    tta.card_number AS transaction_to_account_card_number,
    tta.card_limit AS transaction_to_account_card_limit,
    tta.name_on_card AS transaction_to_account_name_on_card,
    tta.iban AS transaction_to_account_iban,
    tta.note AS transaction_to_account_note,
    tta.is_archived AS transaction_to_account_is_archived,
    tta.exclude_from_balance AS transaction_to_account_exclude_from_balance,
    tta.expiration_date AS transaction_to_account_expiration_date,
    tta.created_at AS transaction_to_account_created_at,
    ttc.string AS transaction_to_account_currency_string,
    ttc.symbol AS transaction_to_account_currency_symbol,
    tcat.id AS transaction_category_id,
    tcat.name AS transaction_category_name,
    tcat.budget_amount AS transaction_category_budget_amount,
    tcat.budget_currency_id AS transaction_category_budget_currency_id,
    tcat.kind_id AS transaction_category_kind_id,
    tcat.created_at AS transaction_category_created_at,
    tk.enum AS transaction_kind_enum,
    tk.increase_balance AS transaction_kind_increase_balance
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


-- name: GetFrequentTxs :many
SELECT
    sqlc.embed(ft),
    sqlc.embed(a),
    sqlc.embed(c),
    sqlc.embed(k),
    cp.name AS counterparty_name,
    ftta.id AS frequent_to_account_id,
    ftta.name AS frequent_to_account_name,
    ftta.kind_id AS frequent_to_account_kind_id,
    ftta.amount AS frequent_to_account_amount,
    ftta.currency_id AS frequent_to_account_currency_id,
    ftta.counterparty_id AS frequent_to_account_counterparty_id,
    ftta.card_number AS frequent_to_account_card_number,
    ftta.card_limit AS frequent_to_account_card_limit,
    ftta.name_on_card AS frequent_to_account_name_on_card,
    ftta.iban AS frequent_to_account_iban,
    ftta.note AS frequent_to_account_note,
    ftta.is_archived AS frequent_to_account_is_archived,
    ftta.exclude_from_balance AS frequent_to_account_exclude_from_balance,
    ftta.expiration_date AS frequent_to_account_expiration_date,
    ftta.created_at AS frequent_to_account_created_at,
    fttc.id AS frequent_to_account_currency_id,
    fttc.string AS frequent_to_account_currency_string,
    fttc.symbol AS frequent_to_account_currency_symbol,
    cat.id AS category_id,
    cat.name AS category_name,
    cat.budget_amount AS category_budget_amount,
    cat.budget_currency_id AS category_budget_currency_id,
    cat.kind_id AS category_kind_id,
    cat.created_at AS category_created_at,
    g.name AS goal_name,
    g.price AS goal_price,
    g.currency_id AS goal_currency_id,
    g.fulfilled_at AS goal_fulfilled_at,
    gc.string AS goal_currency_string,
    gc.symbol AS goal_currency_symbol
FROM frequent_txs AS ft
LEFT JOIN counterparties AS cp ON ft.counterparty_id = cp.id
LEFT JOIN accounts AS a ON ft.account_id = a.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN accounts AS ftta ON ft.to_account_id = a.id
LEFT JOIN currencies AS fttc ON ftta.currency_id = c.id
LEFT JOIN transaction_kinds AS k ON ft.kind_id = k.id
LEFT JOIN categories AS cat ON ft.category_id = cat.id
LEFT JOIN goals AS g ON g.id = ft.goal_id
LEFT JOIN currencies AS gc ON gc.id = g.currency_id;


-- name: GetFrequentTxWTxs :many
SELECT
    sqlc.embed(ft),
    sqlc.embed(a),
    sqlc.embed(c),
    sqlc.embed(k),
    cp.name AS counterparty_name,
    ftta.id AS frequent_to_account_id,
    ftta.name AS frequent_to_account_name,
    ftta.kind_id AS frequent_to_account_kind_id,
    ftta.amount AS frequent_to_account_amount,
    ftta.currency_id AS frequent_to_account_currency_id,
    ftta.counterparty_id AS frequent_to_account_counterparty_id,
    ftta.card_number AS frequent_to_account_card_number,
    ftta.card_limit AS frequent_to_account_card_limit,
    ftta.name_on_card AS frequent_to_account_name_on_card,
    ftta.iban AS frequent_to_account_iban,
    ftta.note AS frequent_to_account_note,
    ftta.is_archived AS frequent_to_account_is_archived,
    ftta.exclude_from_balance AS frequent_to_account_exclude_from_balance,
    ftta.expiration_date AS frequent_to_account_expiration_date,
    ftta.created_at AS frequent_to_account_created_at,
    fttc.id AS frequent_to_account_currency_id,
    fttc.string AS frequent_to_account_currency_string,
    fttc.symbol AS frequent_to_account_currency_symbol,
    cat.id AS category_id,
    cat.name AS category_name,
    cat.budget_amount AS category_budget_amount,
    cat.budget_currency_id AS category_budget_currency_id,
    cat.kind_id AS category_kind_id,
    cat.created_at AS category_created_at,
    g.name AS goal_name,
    g.price AS goal_price,
    g.currency_id AS goal_currency_id,
    g.fulfilled_at AS goal_fulfilled_at,
    gc.string AS goal_currency_string,
    gc.symbol AS goal_currency_symbol,
    t.id AS transaction_id,
    t.account_id AS transaction_account_id,
    t.amount AS transaction_amount,
    t.kind_id AS transaction_kind_id,
    t.category_id AS transaction_category_id,
    t.counterparty_id AS transaction_counterparty_id,
    t.description AS transaction_description,
    t.to_account_id AS transaction_to_account_id,
    t.to_amount AS transaction_to_amount,
    t.goal_id AS transaction_goal_id,
    t.fulfilled_at AS transaction_fulfilled_at,
    t.scheduled_to AS transaction_scheduled_to,
    t.created_at AS transaction_created_at,
    ta.id AS transaction_account_id,
    ta.name AS transaction_account_name,
    ta.kind_id AS transaction_account_kind_id,
    ta.amount AS transaction_account_amount,
    ta.currency_id AS transaction_account_currency_id,
    ta.counterparty_id AS transaction_account_counterparty_id,
    ta.card_number AS transaction_account_card_number,
    ta.card_limit AS transaction_account_card_limit,
    ta.name_on_card AS transaction_account_name_on_card,
    ta.iban AS transaction_account_iban,
    ta.note AS transaction_account_note,
    ta.is_archived AS transaction_account_is_archived,
    ta.exclude_from_balance AS transaction_account_exclude_from_balance,
    ta.expiration_date AS transaction_account_expiration_date,
    ta.created_at AS transaction_account_created_at,
    tc.string AS transaction_currency_string,
    tc.symbol AS transaction_currency_symbol,
    tcp.name AS transaction_counterparty_name,
    tta.id AS transaction_to_account_id,
    tta.name AS transaction_to_account_name,
    tta.kind_id AS transaction_to_account_kind_id,
    tta.amount AS transaction_to_account_amount,
    tta.currency_id AS transaction_to_account_currency_id,
    tta.counterparty_id AS transaction_to_account_counterparty_id,
    tta.card_number AS transaction_to_account_card_number,
    tta.card_limit AS transaction_to_account_card_limit,
    tta.name_on_card AS transaction_to_account_name_on_card,
    tta.iban AS transaction_to_account_iban,
    tta.note AS transaction_to_account_note,
    tta.is_archived AS transaction_to_account_is_archived,
    tta.exclude_from_balance AS transaction_to_account_exclude_from_balance,
    tta.expiration_date AS transaction_to_account_expiration_date,
    tta.created_at AS transaction_to_account_created_at,
    ttc.string AS transaction_to_account_currency_string,
    ttc.symbol AS transaction_to_account_currency_symbol,
    tcat.id AS transaction_category_id,
    tcat.name AS transaction_category_name,
    tcat.budget_amount AS transaction_category_budget_amount,
    tcat.budget_currency_id AS transaction_category_budget_currency_id,
    tcat.kind_id AS transaction_category_kind_id,
    tcat.created_at AS transaction_category_created_at,
    tk.enum AS transaction_kind_enum,
    tk.increase_balance AS transaction_kind_increase_balance
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


-- name: GetFrequentTx :one
SELECT
    sqlc.embed(ft),
    sqlc.embed(a),
    sqlc.embed(c),
    sqlc.embed(k),
    cp.name AS counterparty_name,
    ftta.id AS frequent_to_account_id,
    ftta.name AS frequent_to_account_name,
    ftta.kind_id AS frequent_to_account_kind_id,
    ftta.amount AS frequent_to_account_amount,
    ftta.currency_id AS frequent_to_account_currency_id,
    ftta.counterparty_id AS frequent_to_account_counterparty_id,
    ftta.card_number AS frequent_to_account_card_number,
    ftta.card_limit AS frequent_to_account_card_limit,
    ftta.name_on_card AS frequent_to_account_name_on_card,
    ftta.iban AS frequent_to_account_iban,
    ftta.note AS frequent_to_account_note,
    ftta.is_archived AS frequent_to_account_is_archived,
    ftta.exclude_from_balance AS frequent_to_account_exclude_from_balance,
    ftta.expiration_date AS frequent_to_account_expiration_date,
    ftta.created_at AS frequent_to_account_created_at,
    fttc.id AS frequent_to_account_currency_id,
    fttc.string AS frequent_to_account_currency_string,
    fttc.symbol AS frequent_to_account_currency_symbol,
    cat.id AS category_id,
    cat.name AS category_name,
    cat.budget_amount AS category_budget_amount,
    cat.budget_currency_id AS category_budget_currency_id,
    cat.kind_id AS category_kind_id,
    cat.created_at AS category_created_at,
    g.name AS goal_name,
    g.price AS goal_price,
    g.currency_id AS goal_currency_id,
    g.fulfilled_at AS goal_fulfilled_at,
    gc.string AS goal_currency_string,
    gc.symbol AS goal_currency_symbol
FROM frequent_txs AS ft
LEFT JOIN counterparties AS cp ON ft.counterparty_id = cp.id
LEFT JOIN accounts AS a ON ft.account_id = a.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN accounts AS ftta ON ft.to_account_id = a.id
LEFT JOIN currencies AS fttc ON ftta.currency_id = c.id
LEFT JOIN transaction_kinds AS k ON ft.kind_id = k.id
LEFT JOIN categories AS cat ON ft.category_id = cat.id
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
