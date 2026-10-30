-- name: CreateGamyaa :exec
INSERT INTO gamyaas (
    account_id,
    installment,
    members,
    shares,
    turns,
    starts_on,
    pay_from_account_id,
    recurrent_tx_id
)
VALUES (
    sqlc.arg(account_id),
    sqlc.arg(installment),
    sqlc.arg(members),
    sqlc.arg(shares),
    sqlc.arg(turns),
    sqlc.arg(starts_on),
    sqlc.arg(pay_from_account_id),
    sqlc.arg(recurrent_tx_id)
);

-- name: GetGamyaas :many
SELECT
    sqlc.embed(g),
    pa.name AS pay_from_account_name
FROM gamyaas AS g
LEFT JOIN accounts AS pa ON g.pay_from_account_id = pa.id;

-- name: GetGamyaaTransactions :many
-- Transfers into gam'yaas (installments) and out of them (payouts), oldest
-- first.
SELECT
    t.account_id,
    t.to_account_id,
    t.amount,
    t.to_amount,
    t.scheduled_to,
    t.fulfilled_at
FROM transactions AS t
WHERE
    t.account_id IN (SELECT account_id FROM gamyaas)
    OR t.to_account_id IN (SELECT account_id FROM gamyaas)
ORDER BY t.scheduled_to;
