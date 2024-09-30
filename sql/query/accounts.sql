-- name: GetAccounts :many
WITH gls_amts AS (
    SELECT
        SUM(t.amount) AS amount,
        t.account_id
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
    a.card_number,
    a.iban,
    a.note,
    a.exclude_from_balance,
    a.is_archived,
    a.is_default,
    a.created_at,
    c.enum AS currency_enum,
    c.egp_fx_rate,
    k.enum AS kind_enum,
    IFNULL(a.amount + gla.amount, a.amount) AS actual_amount
FROM accounts AS a
LEFT JOIN account_kinds AS k ON a.kind_id = k.id
LEFT JOIN currencies AS c ON c.id = a.currency_id
LEFT JOIN gls_amts AS gla ON gla.account_id = a.id
ORDER BY a.created_at DESC;

-- name: GetActiveAccounts :many
WITH gls_amts AS (
    SELECT
        SUM(t.amount) AS amount,
        t.account_id
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
    a.card_number,
    a.iban,
    a.note,
    a.exclude_from_balance,
    a.is_archived,
    a.is_default,
    a.created_at,
    c.enum AS currency_enum,
    c.egp_fx_rate,
    k.enum AS kind_enum,
    IFNULL(a.amount + gla.amount, a.amount) AS actual_amount
FROM accounts AS a
LEFT JOIN account_kinds AS k ON a.kind_id = k.id
LEFT JOIN currencies AS c ON c.id = a.currency_id
LEFT JOIN gls_amts AS gla ON gla.account_id = a.id
WHERE a.is_archived IS FALSE
ORDER BY a.created_at DESC;

-- name: GetAccount :one
WITH gls_amts AS (
    SELECT
        SUM(t.amount) AS amount,
        t.account_id
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
    a.card_number,
    a.iban,
    a.note,
    a.exclude_from_balance,
    a.is_archived,
    a.is_default,
    a.created_at,
    c.enum AS currency_enum,
    c.egp_fx_rate,
    k.enum AS kind_enum,
    IFNULL(a.amount + gla.amount, a.amount) AS actual_amount
FROM accounts AS a
LEFT JOIN account_kinds AS k ON a.kind_id = k.id
LEFT JOIN currencies AS c ON c.id = a.currency_id
LEFT JOIN gls_amts AS gla ON gla.account_id = a.id
WHERE a.id = ?;

-- name: CreateAccount :exec
INSERT INTO accounts (
    id,
    name,
    kind_id,
    amount,
    currency_id,
    card_number,
    iban,
    note,
    exclude_from_balance,
    is_archived,
    is_default
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateAccount :exec
UPDATE accounts
SET
name = ?,
kind_id = ?,
currency_id = ?,
card_number = ?,
iban = ?,
note = ?,
is_archived = ?,
exclude_from_balance = ?
WHERE id = ?;

-- name: UnsetDefaultAccountExpect :exec
UPDATE accounts
SET
is_default = FALSE
WHERE id != ?;

-- name: SetDefaultAccount :exec
UPDATE accounts
SET
is_default = TRUE
WHERE id = ?;


-- name: DeleteAccount :exec
DELETE FROM accounts
WHERE id = ?;

-- name: AddToAccount :exec
UPDATE accounts SET amount = amount + ? WHERE id = ?;

-- name: RemoveFromAccount :exec
UPDATE accounts SET amount = amount - ? WHERE id = ?;

-- name: GetBalances :many
SELECT
    a.currency_id,
    c.enum AS currency_enum,
    c.egp_fx_rate,
    TOTAL(a.amount) AS balance
FROM accounts AS a
LEFT JOIN currencies AS c ON a.currency_id = c.id
WHERE a.is_archived IS FALSE
    AND a.exclude_from_balance IS FALSE
GROUP BY currency_id
ORDER BY balance DESC;
