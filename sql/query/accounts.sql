-- name: GetAccounts :many
WITH gls_amts AS (
    SELECT
        t.account_id,
        SUM(t.amount) AS amount
    FROM goals AS gl
    LEFT JOIN goals_progresses AS gp ON gl.id = gp.goal_id
    LEFT JOIN transactions AS t ON gp.transaction_id = t.id
    WHERE gl.fulfilled_at IS NULL
    GROUP BY t.amount
)
SELECT
    a.id,
    a.name,
    a.kind_id,
    a.amount,
    a.currency_id,
    a.counterparty_id,
    a.card_number,
    a.card_limit,
    a.name_on_card,
    a.iban,
    a.note,
    a.exclude_from_balance,
    a.is_archived,
    a.is_default,
    a.expiration_date,
    a.created_at,
    c.enum AS currency_enum,
    k.enum AS kind_enum,
    cp.name AS counterparty_name,
    IFNULL(a.amount + gla.amount, a.amount) AS actual_amount
FROM accounts AS a
LEFT JOIN account_kinds AS k ON a.kind_id = k.id
LEFT JOIN counterparties AS cp ON a.counterparty_id = cp.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN gls_amts AS gla ON a.id = gla.account_id
ORDER BY a.created_at DESC;

-- name: GetActiveAccounts :many
WITH gls_amts AS (
    SELECT
        t.account_id,
        SUM(t.amount) AS amount
    FROM goals AS gl
    LEFT JOIN goals_progresses AS gp ON gl.id = gp.goal_id
    LEFT JOIN transactions AS t ON gp.transaction_id = t.id
    WHERE gl.fulfilled_at IS NULL
    GROUP BY t.amount
)
SELECT
    a.id,
    a.name,
    a.kind_id,
    a.amount,
    a.currency_id,
    a.counterparty_id,
    a.card_number,
    a.card_limit,
    a.name_on_card,
    a.iban,
    a.note,
    a.exclude_from_balance,
    a.is_archived,
    a.is_default,
    a.expiration_date,
    a.created_at,
    c.enum AS currency_enum,
    k.enum AS kind_enum,
    cp.name AS counterparty_name,
    IFNULL(a.amount + gla.amount, a.amount) AS actual_amount
FROM accounts AS a
LEFT JOIN account_kinds AS k ON a.kind_id = k.id
LEFT JOIN counterparties AS cp ON a.counterparty_id = cp.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN gls_amts AS gla ON a.id = gla.account_id
WHERE a.is_archived IS FALSE
ORDER BY a.created_at DESC;

-- name: GetAccount :one
WITH gls_amts AS (
    SELECT
        t.account_id,
        SUM(t.amount) AS amount
    FROM goals AS gl
    LEFT JOIN goals_progresses AS gp ON gl.id = gp.goal_id
    LEFT JOIN transactions AS t ON gp.transaction_id = t.id
    WHERE gl.fulfilled_at IS NULL
    GROUP BY t.amount
)
SELECT
    a.id,
    a.name,
    a.kind_id,
    a.amount,
    a.currency_id,
    a.counterparty_id,
    a.card_number,
    a.card_limit,
    a.name_on_card,
    a.iban,
    a.note,
    a.exclude_from_balance,
    a.is_archived,
    a.is_default,
    a.expiration_date,
    a.created_at,
    c.enum AS currency_enum,
    k.enum AS kind_enum,
    cp.name AS counterparty_name,
    IFNULL(a.amount + gla.amount, a.amount) AS actual_amount
FROM accounts AS a
LEFT JOIN account_kinds AS k ON a.kind_id = k.id
LEFT JOIN counterparties AS cp ON a.counterparty_id = cp.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN gls_amts AS gla ON a.id = gla.account_id
WHERE a.id = sqlc.arg(id);

-- name: CreateAccount :exec
INSERT INTO accounts (
    id,
    name,
    kind_id,
    amount,
    currency_id,
    counterparty_id,
    card_number,
    card_limit,
    name_on_card,
    iban,
    note,
    exclude_from_balance,
    is_archived,
    is_default,
    expiration_date
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(name),
    sqlc.arg(kind_id),
    sqlc.arg(amount),
    sqlc.arg(currency_id),
    sqlc.arg(counterparty_id),
    sqlc.arg(card_number),
    sqlc.arg(card_limit),
    sqlc.arg(name_on_card),
    sqlc.arg(iban),
    sqlc.arg(note),
    sqlc.arg(exclude_from_balance),
    sqlc.arg(is_archived),
    sqlc.arg(is_default),
    sqlc.arg(expiration_date)
);

-- name: UpdateAccount :exec
UPDATE accounts
SET
    name = sqlc.arg(name),
    kind_id = sqlc.arg(kind_id),
    currency_id = sqlc.arg(currency_id),
    counterparty_id = sqlc.arg(counterparty_id),
    card_number = sqlc.arg(card_number),
    card_limit = sqlc.arg(card_limit),
    name_on_card = sqlc.arg(name_on_card),
    iban = sqlc.arg(iban),
    note = sqlc.arg(note),
    is_archived = sqlc.arg(is_archived),
    exclude_from_balance = sqlc.arg(exclude_from_balance),
    expiration_date = sqlc.arg(expiration_date)
WHERE id = sqlc.arg(id);

-- name: UnsetDefaultAccountExpect :exec
UPDATE accounts
SET
    is_default = FALSE
WHERE id != sqlc.arg(id);

-- name: SetDefaultAccount :exec
UPDATE accounts
SET
    is_default = TRUE
WHERE id = sqlc.arg(id);


-- name: DeleteAccount :exec
DELETE FROM accounts
WHERE id = sqlc.arg(id);

-- name: AddToAccount :exec
UPDATE accounts
SET
    amount = ROUND(amount + sqlc.arg(amount), 2)
WHERE id = sqlc.arg(id);

-- name: RemoveFromAccount :exec
UPDATE accounts
SET
    amount = ROUND(amount - sqlc.arg(amount), 2)
WHERE id = sqlc.arg(id);

-- name: GetBalances :many
SELECT
    a.currency_id,
    c.enum AS currency_enum,
    TOTAL(a.amount) AS balance
FROM accounts AS a
LEFT JOIN currencies AS c ON a.currency_id = c.id
WHERE
    a.is_archived IS FALSE
    AND a.exclude_from_balance IS FALSE
GROUP BY a.currency_id
ORDER BY c.enum;
