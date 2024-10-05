-- name: GetAccountKinds :many
SELECT
    id,
    enum
FROM account_kinds
ORDER BY enum;

-- name: CreateAccountKind :exec
INSERT INTO account_kinds (id, enum)
VALUES (
    sqlc.arg(id),
    sqlc.arg(enum)
);
