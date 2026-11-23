-- name: GetTags :many
-- Every tag with how many transactions carry it, archived ones last, then the
-- trips by their first day (latest first), then by name.
SELECT
    sqlc.embed(tags),
    (SELECT COUNT(*) FROM transaction_tags AS tt WHERE tt.tag_id = tags.id) AS txs
FROM tags
ORDER BY
    tags.archived_at IS NOT NULL,
    tags.starts_on IS NULL,
    tags.starts_on DESC,
    tags.name COLLATE NOCASE;

-- name: GetTag :one
SELECT
    sqlc.embed(tags),
    (SELECT COUNT(*) FROM transaction_tags AS tt WHERE tt.tag_id = tags.id) AS txs
FROM tags
WHERE tags.id = sqlc.arg(id);

-- name: GetTagByName :one
SELECT * FROM tags WHERE name = sqlc.arg(name) COLLATE NOCASE;

-- name: CreateTag :exec
INSERT INTO tags (
    id, name, color, starts_on, ends_on, auto_tag, budget, budget_currency_id, note
) VALUES (
    sqlc.arg(id), sqlc.arg(name), sqlc.arg(color), sqlc.narg(starts_on), sqlc.narg(ends_on),
    sqlc.arg(auto_tag), sqlc.narg(budget), sqlc.narg(budget_currency_id), sqlc.arg(note)
);

-- name: UpdateTag :exec
UPDATE tags SET
    name = sqlc.arg(name),
    color = sqlc.arg(color),
    starts_on = sqlc.narg(starts_on),
    ends_on = sqlc.narg(ends_on),
    auto_tag = sqlc.arg(auto_tag),
    budget = sqlc.narg(budget),
    budget_currency_id = sqlc.narg(budget_currency_id),
    note = sqlc.arg(note)
WHERE id = sqlc.arg(id);

-- name: SetTagArchived :exec
UPDATE tags SET archived_at = sqlc.narg(archived_at) WHERE id = sqlc.arg(id);

-- name: DeleteTag :exec
-- Its transactions stay, untagged (ON DELETE CASCADE of the links).
DELETE FROM tags WHERE id = sqlc.arg(id);

-- name: GetAutoTags :many
-- The live tags tagging new transactions dated in their days.
SELECT * FROM tags
WHERE auto_tag = 1 AND archived_at IS NULL AND starts_on IS NOT NULL AND ends_on IS NOT NULL;

-- name: GetTagsOfTxs :many
-- The tags of the transactions, by name.
SELECT tt.transaction_id, sqlc.embed(tags)
FROM transaction_tags AS tt
JOIN tags ON tags.id = tt.tag_id
WHERE tt.transaction_id IN (sqlc.slice(ids))
ORDER BY tags.name COLLATE NOCASE;

-- name: AddTransactionTag :exec
INSERT OR IGNORE INTO transaction_tags (transaction_id, tag_id)
VALUES (sqlc.arg(transaction_id), sqlc.arg(tag_id));

-- name: RemoveTransactionTag :exec
DELETE FROM transaction_tags
WHERE transaction_id = sqlc.arg(transaction_id) AND tag_id = sqlc.arg(tag_id);

-- name: DeleteTransactionTags :exec
DELETE FROM transaction_tags WHERE transaction_id = sqlc.arg(transaction_id);

-- name: GetTagRangeCandidates :many
-- The transactions dated in [from_day, to_day] a trip can tag at once: not
-- tagged with it yet, no transfers (between own accounts or a transfer pair's
-- legs), balance corrections, goals' money or loan payments, nor deleted
-- accounts'. Signed as they move the balance, in the account's currency.
SELECT
    t.id,
    sqlc.embed(cur),
    CAST(IIF(t.kind IN (1, 4), -t.amount, t.amount) AS FLOAT) AS amount
FROM transactions AS t
JOIN accounts AS a ON t.account_id = a.id
JOIN currencies AS cur ON a.currency_id = cur.id
WHERE
    SUBSTR(t.scheduled_to, 1, 10) >= CAST(sqlc.arg(from_day) AS TEXT)
    AND SUBSTR(t.scheduled_to, 1, 10) <= CAST(sqlc.arg(to_day) AS TEXT)
    AND t.kind NOT IN (3, 6)
    AND t.transfer_pair_id IS NULL
    AND t.loan_goal_id IS NULL
    AND a.goal_id IS NULL
    AND a.deleted_at IS NULL
    AND NOT EXISTS (
        SELECT 1 FROM accounts AS ga WHERE ga.id = t.to_account_id AND ga.goal_id IS NOT NULL
    )
    AND NOT EXISTS (
        SELECT 1 FROM transaction_tags AS tt WHERE tt.transaction_id = t.id AND tt.tag_id = sqlc.arg(tag_id)
    )
ORDER BY t.scheduled_to;

-- name: GetTagSpending :many
-- What the tagged fulfilled expenses spent, following the analysis rule (net
-- of their reimbursements, refunds subtracting, no reimbursements nor
-- transfer pair legs), per tag, day (scheduled_to's), category and currency.
SELECT
    tt.tag_id,
    CAST(SUBSTR(t.scheduled_to, 1, 10) AS TEXT) AS day,
    CAST(IFNULL(t.category_id, '') AS TEXT) AS category_id,
    sqlc.embed(cur),
    CAST(TOTAL(IIF(
        t.kind = 1,
        t.amount - (
            SELECT TOTAL(rt.amount) FROM transactions AS rt WHERE rt.reimburses_id = t.id
        ),
        -t.amount
    )) AS FLOAT) AS total
FROM transaction_tags AS tt
JOIN transactions AS t ON t.id = tt.transaction_id
JOIN accounts AS a ON t.account_id = a.id
JOIN currencies AS cur ON a.currency_id = cur.id
WHERE
    t.kind IN (1, 5)
    AND t.reimburses_id IS NULL
    AND t.transfer_pair_id IS NULL
    AND t.fulfilled_at IS NOT NULL
    AND (sqlc.narg(tag_id) IS NULL OR tt.tag_id = sqlc.narg(tag_id))
GROUP BY tt.tag_id, day, t.category_id, cur.id
ORDER BY day;
