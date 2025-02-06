-- name: GetAccountKinds :many
SELECT
    sqlc.embed(account_kinds)
FROM account_kinds
ORDER BY enum;

-- name: CreateAccountKind :exec
INSERT INTO account_kinds (id, enum)
VALUES (
    sqlc.arg(id),
    sqlc.arg(enum)
);
