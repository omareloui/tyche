-- name: GetBalances :many
SELECT
    b.id,
    b.amount,
    b.currency_id,
    c.enum,
    c.egp_fx_rate
FROM balances AS b
LEFT JOIN currencies AS c ON b.currency_id = c.id;

-- name: CreateBalance :exec
INSERT INTO balances (id, currency_id, amount) VALUES (?, ?, ?);

-- name: AddToBalance :exec
UPDATE balances SET amount = amount + ? WHERE currency_id = ?;

-- name: RemoveFromBalance :exec
UPDATE balances SET amount = amount - ? WHERE currency_id = ?;
