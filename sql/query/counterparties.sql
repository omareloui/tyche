-- name: CreateCounterparty :exec
INSERT INTO counterparties (
    id,
    name,
    aliases,
    icon,
    color,
    is_merchant,
    default_category_id,
    rating,
    note
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(name),
    sqlc.arg(aliases),
    sqlc.arg(icon),
    sqlc.arg(color),
    sqlc.arg(is_merchant),
    sqlc.arg(default_category_id),
    sqlc.narg(rating),
    sqlc.arg(note)
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
    default_category_id = sqlc.arg(default_category_id),
    is_archived = sqlc.arg(is_archived),
    rating = sqlc.narg(rating),
    note = sqlc.arg(note)
WHERE id = sqlc.arg(id);

-- name: SetCounterpartyAppearance :exec
UPDATE counterparties
SET
    icon = sqlc.arg(icon),
    color = sqlc.arg(color)
WHERE id = sqlc.arg(id);

-- name: SetCounterpartyArchived :exec
UPDATE counterparties
SET is_archived = sqlc.arg(is_archived)
WHERE id = sqlc.arg(id);

-- name: SetCounterpartyRating :exec
UPDATE counterparties
SET rating = sqlc.narg(rating)
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

-- name: ReassignCounterpartyLocations :exec
-- Appended after the into counterparty's offset own locations.
UPDATE counterparty_locations
SET
    counterparty_id = sqlc.arg(to_id),
    position = position + sqlc.arg(offset)
WHERE counterparty_id = sqlc.arg(from_id);

-- name: GetCounterpartyLocations :many
SELECT * FROM counterparty_locations
ORDER BY counterparty_id, position;

-- name: GetCounterpartyLocationsOf :many
SELECT * FROM counterparty_locations
WHERE counterparty_id = sqlc.arg(counterparty_id)
ORDER BY position;

-- name: CreateCounterpartyLocation :exec
INSERT INTO counterparty_locations (id, counterparty_id, label, address, maps_url, position)
VALUES (
    sqlc.arg(id),
    sqlc.arg(counterparty_id),
    sqlc.arg(label),
    sqlc.arg(address),
    sqlc.arg(maps_url),
    sqlc.arg(position)
);

-- name: DeleteCounterpartyLocations :exec
DELETE FROM counterparty_locations
WHERE counterparty_id = sqlc.arg(counterparty_id);
