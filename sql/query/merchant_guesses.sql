-- name: GetMerchantGuesses :many
SELECT * FROM merchant_guesses ORDER BY created_at;

-- name: GetMerchantGuess :one
SELECT * FROM merchant_guesses WHERE id = sqlc.arg(id);

-- name: CreateMerchantGuess :exec
INSERT INTO merchant_guesses (id, raw, counterparty_id, category_id, created)
VALUES (sqlc.arg(id), sqlc.arg(raw), sqlc.arg(counterparty_id), sqlc.narg(category_id), sqlc.arg(created));

-- name: RejectMerchantGuess :exec
UPDATE merchant_guesses SET rejected_at = CURRENT_TIMESTAMP WHERE id = sqlc.arg(id);

-- name: ClearTransactionsMerchantGuess :exec
UPDATE transactions SET merchant_guess_id = NULL WHERE merchant_guess_id = sqlc.arg(merchant_guess_id);

-- name: SetTransactionMerchantGuess :exec
UPDATE transactions SET merchant_guess_id = sqlc.arg(merchant_guess_id) WHERE id = sqlc.arg(id);

-- name: GetTransactionMerchantGuess :one
SELECT g.*
FROM transactions AS t
JOIN merchant_guesses AS g ON g.id = t.merchant_guess_id
WHERE t.id = sqlc.arg(id) AND g.rejected_at IS NULL;

-- name: SetCounterpartyAliases :exec
UPDATE counterparties SET aliases = sqlc.arg(aliases) WHERE id = sqlc.arg(id);

-- name: GetAIMerchants :one
SELECT ai_merchants FROM user LIMIT 1;

-- name: SetAIMerchants :exec
UPDATE user SET ai_merchants = sqlc.arg(ai_merchants);
