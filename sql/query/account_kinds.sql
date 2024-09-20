-- name: GetAccountKinds :many
SELECT id, enum FROM account_kinds;

-- name: CreateAccountKind :exec
INSERT INTO account_kinds (id, enum)
VALUES (?, ?);
