-- name: GetAccounts :many
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
    a.created_at,
    c.enum AS currency_enum,
    c.egp_fx_rate,
    k.enum AS kind_enum
FROM accounts AS a
LEFT JOIN account_kinds AS k ON a.kind_id = k.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
ORDER BY a.created_at DESC;

-- name: GetAccount :one
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
    a.created_at,
    c.enum AS currency_enum,
    c.egp_fx_rate,
    k.enum AS kind_enum
FROM accounts AS a
LEFT JOIN account_kinds AS k ON a.kind_id = k.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
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
    created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateAccount :exec
UPDATE accounts
SET
name = ?,
kind_id = ?,
amount = ?,
currency_id = ?,
card_number = ?,
iban = ?,
note = ?,
is_archived = ?,
exclude_from_balance = ?
WHERE id = ?;

-- name: AddToAccount :exec
UPDATE accounts SET amount = amount + ? WHERE id = ?;

-- name: RemoveFromAccount :exec
UPDATE accounts SET amount = amount - ? WHERE id = ?;
