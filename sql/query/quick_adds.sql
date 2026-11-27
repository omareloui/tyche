-- name: CreateQuickAdd :exec
INSERT INTO quick_adds (id, text, model) VALUES (sqlc.arg(id), sqlc.arg(text), sqlc.arg(model));

-- name: SetTransactionQuickAdd :exec
UPDATE transactions SET quick_add_id = sqlc.arg(quick_add_id) WHERE id = sqlc.arg(id);

-- name: GetTransactionQuickAddText :one
SELECT q.text
FROM transactions AS t
JOIN quick_adds AS q ON q.id = t.quick_add_id
WHERE t.id = sqlc.arg(id);

-- name: GetAIQuickAdd :one
SELECT ai_quick_add FROM user LIMIT 1;

-- name: SetAIQuickAdd :exec
UPDATE user SET ai_quick_add = sqlc.arg(ai_quick_add);

-- name: GetCounterpartyUses :many
-- How many transactions name each counterparty (or move its debt account),
-- to send the most used ones.
SELECT CAST(u.id AS TEXT) AS id, CAST(SUM(u.n) AS INTEGER) AS uses
FROM (
    SELECT t.counterparty_id AS id, COUNT(*) AS n
    FROM transactions AS t
    WHERE t.counterparty_id IS NOT NULL
    GROUP BY t.counterparty_id
    UNION ALL
    SELECT a.counterparty_id AS id, COUNT(*) AS n
    FROM transactions AS t
    JOIN accounts AS a ON a.id IN (t.account_id, t.to_account_id)
    WHERE a.counterparty_id IS NOT NULL
    GROUP BY a.counterparty_id
) AS u
GROUP BY u.id;
