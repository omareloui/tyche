-- name: CreateCounterparty :exec
INSERT INTO counterparties (
    id,
    name,
    aliases,
    icon,
    color,
    is_merchant,
    default_category_id
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(name),
    sqlc.arg(aliases),
    sqlc.arg(icon),
    sqlc.arg(color),
    sqlc.arg(is_merchant),
    sqlc.arg(default_category_id)
);

-- name: GetCounterparties :many
SELECT
    sqlc.embed(counterparties),
    cat.name AS default_category_name,
    cat.color AS default_category_color,
    cat.kinds AS default_category_kinds
FROM counterparties
LEFT JOIN categories AS cat ON counterparties.default_category_id = cat.id
ORDER BY counterparties.name COLLATE NOCASE;

-- name: GetCounterparty :one
SELECT
    sqlc.embed(counterparties),
    cat.name AS default_category_name,
    cat.color AS default_category_color,
    cat.kinds AS default_category_kinds
FROM counterparties
LEFT JOIN categories AS cat ON counterparties.default_category_id = cat.id
WHERE counterparties.id = sqlc.arg(id);

-- name: UpdateCounterparty :exec
UPDATE counterparties
SET
    name = sqlc.arg(name),
    aliases = sqlc.arg(aliases),
    icon = sqlc.arg(icon),
    color = sqlc.arg(color),
    is_merchant = sqlc.arg(is_merchant),
    default_category_id = sqlc.arg(default_category_id)
WHERE id = sqlc.arg(id);

-- name: SetCounterpartyAppearance :exec
UPDATE counterparties
SET
    icon = sqlc.arg(icon),
    color = sqlc.arg(color)
WHERE id = sqlc.arg(id);

-- name: DeleteCounterparty :exec
DELETE FROM counterparties
WHERE id = sqlc.arg(id);

-- name: ReassignTransactionsCounterparty :exec
UPDATE transactions
SET counterparty_id = sqlc.arg(to_id)
WHERE counterparty_id = sqlc.arg(from_id);

-- name: ReassignRecurrentTxsCounterparty :exec
UPDATE recurrent_txs
SET counterparty_id = sqlc.arg(to_id)
WHERE counterparty_id = sqlc.arg(from_id);

-- name: ReassignAccountsCounterparty :exec
UPDATE accounts
SET counterparty_id = sqlc.arg(to_id)
WHERE counterparty_id = sqlc.arg(from_id);
