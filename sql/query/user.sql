-- name: GetUserByOAuthID :one
SELECT
    sqlc.embed(user),
    sqlc.embed(currencies),
    accounts.id AS account_id,
    accounts.name AS account_name,
    accounts.kind_id AS account_kind_id,
    accounts.amount AS account_amount,
    accounts.currency_id AS account_currency_id,
    accounts.counterparty_id AS account_counterparty_id,
    accounts.card_number AS account_card_number,
    accounts.card_limit AS account_card_limit,
    accounts.name_on_card AS account_name_on_card,
    accounts.iban AS account_iban,
    accounts.note AS account_note,
    accounts.is_archived AS account_is_archived,
    accounts.exclude_from_balance AS account_exclude_from_balance,
    accounts.expiration_date AS account_expiration_date,
    accounts.created_at AS account_created_at
FROM user
LEFT JOIN currencies ON user.default_currency_id = currencies.id
LEFT JOIN accounts ON user.default_account_id = accounts.id
WHERE user.oauth_id = sqlc.arg(oauth_id) LIMIT 1;


-- name: UpdateSeededAt :exec
UPDATE user
SET
    seeded_at = DATETIME('now')
WHERE oauth_id = sqlc.arg(oauth_id);

-- name: SetDefaultAccount :exec
UPDATE user
SET default_account_id = sqlc.arg(account_id);

-- name: SetDefaultCurrency :exec
UPDATE user
SET default_currency_id = sqlc.arg(currency_id);

-- name: CreateUser :exec
INSERT INTO user (
    id,
    oauth_id,
    oauth_provider,
    email,
    verified_email,
    first_name,
    last_name,
    picture,
    default_currency_id,
    start_of_period
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(oauth_id),
    sqlc.arg(oauth_provider),
    sqlc.arg(email),
    sqlc.arg(verified_email),
    sqlc.arg(first_name),
    sqlc.arg(last_name),
    sqlc.arg(picture),
    sqlc.arg(default_currency_id),
    sqlc.arg(start_of_period)
);

-- name: GetWorkSettings :one
SELECT
    user.monthly_salary,
    user.work_hours_per_week,
    c.id AS currency_id,
    c.string AS currency_string,
    c.symbol AS currency_symbol
FROM user
LEFT JOIN currencies AS c ON c.id = COALESCE(user.salary_currency_id, user.default_currency_id)
LIMIT 1;

-- name: SetWorkSettings :exec
UPDATE user
SET
    monthly_salary = sqlc.arg(monthly_salary),
    salary_currency_id = sqlc.arg(salary_currency_id),
    work_hours_per_week = sqlc.arg(work_hours_per_week);
