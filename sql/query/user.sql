-- name: GetUserByOAuthID :one
SELECT
    sqlc.embed(user),
    sqlc.embed(currencies)
FROM user
LEFT JOIN currencies ON user.default_currency_id = currencies.id
WHERE oauth_id = sqlc.arg(oauth_id) LIMIT 1;


-- name: UpdateSeededAt :exec
UPDATE user
SET
    seeded_at = DATETIME('now')
WHERE oauth_id = sqlc.arg(oauth_id);



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
