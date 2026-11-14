-- name: CreateRecurrentTx :exec
INSERT INTO recurrent_txs (
    id,
    every,
    unit,
    account_id,
    amount,
    kind,
    category_id,
    counterparty_id,
    description,
    note,
    to_account_id,
    to_amount,
    starts_at,
    stops_at,
    anchor,
    next_index,
    next_on,
    auto_fulfill
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(every),
    sqlc.arg(unit),
    sqlc.arg(account_id),
    ROUND(sqlc.arg(amount), 6),
    sqlc.arg(kind),
    sqlc.arg(category_id),
    sqlc.arg(counterparty_id),
    sqlc.arg(description),
    sqlc.arg(note),
    sqlc.arg(to_account_id),
    IIF(sqlc.narg(to_amount) > 0.0, ROUND(sqlc.narg(to_amount), 6), NULL),
    sqlc.arg(starts_at),
    sqlc.arg(stops_at),
    sqlc.arg(anchor),
    sqlc.arg(next_index),
    sqlc.arg(next_on),
    sqlc.arg(auto_fulfill)
);



-- name: GetRecurrentTxs :many
SELECT
    sqlc.embed(rtd),
    ak.enum AS account_kind_enum,
    tak.enum AS to_account_kind_enum,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    cp.is_merchant AS counterparty_is_merchant,
    cat.color AS category_color
FROM recurrent_txs_details AS rtd
LEFT JOIN account_kinds AS ak ON rtd.account_kind_id = ak.id
LEFT JOIN account_kinds AS tak ON rtd.to_account_kind_id = tak.id
LEFT JOIN counterparties AS cp ON rtd.recurrent_tx_counterparty_id = cp.id
LEFT JOIN categories AS cat ON rtd.recurrent_tx_category_id = cat.id
ORDER BY rtd.recurrent_tx_created_at DESC;


-- name: GetRecurrentTx :one
SELECT
    sqlc.embed(rtd),
    ak.enum AS account_kind_enum,
    tak.enum AS to_account_kind_enum,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    cp.is_merchant AS counterparty_is_merchant,
    cat.color AS category_color
FROM recurrent_txs_details AS rtd
LEFT JOIN account_kinds AS ak ON rtd.account_kind_id = ak.id
LEFT JOIN account_kinds AS tak ON rtd.to_account_kind_id = tak.id
LEFT JOIN counterparties AS cp ON rtd.recurrent_tx_counterparty_id = cp.id
LEFT JOIN categories AS cat ON rtd.recurrent_tx_category_id = cat.id
WHERE rtd.recurrent_tx_id = sqlc.arg(id);


-- name: UpdateRecurrentTx :exec
UPDATE recurrent_txs
SET
    every = sqlc.arg(every),
    unit = sqlc.arg(unit),
    account_id = sqlc.arg(account_id),
    amount = ROUND(sqlc.arg(amount), 6),
    kind = sqlc.arg(kind),
    category_id = sqlc.arg(category_id),
    counterparty_id = sqlc.arg(counterparty_id),
    description = sqlc.arg(description),
    note = sqlc.arg(note),
    to_account_id = sqlc.arg(to_account_id),
    to_amount = IIF(sqlc.narg(to_amount) > 0.0, ROUND(sqlc.narg(to_amount), 6), NULL),
    stops_at = sqlc.arg(stops_at),
    anchor = sqlc.arg(anchor),
    next_index = sqlc.arg(next_index),
    next_on = sqlc.arg(next_on),
    auto_fulfill = sqlc.arg(auto_fulfill)
WHERE id = sqlc.arg(id);


-- name: SetRecurrentTxNext :exec
-- Saves how far its occurrences are written: the next one's index and date.
UPDATE recurrent_txs
SET
    next_index = sqlc.arg(next_index),
    next_on = sqlc.arg(next_on)
WHERE id = sqlc.arg(id);

-- name: SetRecurrentTxPaused :exec
UPDATE recurrent_txs
SET
    paused_at = sqlc.narg(paused_at),
    next_index = sqlc.arg(next_index),
    next_on = sqlc.arg(next_on)
WHERE id = sqlc.arg(id);

-- name: GetRecurrentTxsDue :many
-- The recurrent txs that may have occurrences to write: not paused nor past
-- their end (next_on NULL: not computed yet).
SELECT
    sqlc.embed(rtd),
    ak.enum AS account_kind_enum,
    tak.enum AS to_account_kind_enum,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    cp.is_merchant AS counterparty_is_merchant,
    cat.color AS category_color
FROM recurrent_txs_details AS rtd
LEFT JOIN account_kinds AS ak ON rtd.account_kind_id = ak.id
LEFT JOIN account_kinds AS tak ON rtd.to_account_kind_id = tak.id
LEFT JOIN counterparties AS cp ON rtd.recurrent_tx_counterparty_id = cp.id
LEFT JOIN categories AS cat ON rtd.recurrent_tx_category_id = cat.id
WHERE rtd.recurrent_tx_paused_at IS NULL
    AND (
        rtd.recurrent_tx_next_on IS NULL
        OR rtd.recurrent_tx_stops_at IS NULL
        OR rtd.recurrent_tx_next_on <= rtd.recurrent_tx_stops_at
    );

-- name: UpdateUnfulfilledOccurrences :exec
-- Applies a recurrent tx's edited fields to its scheduled occurrences, but the
-- ones edited on their own.
UPDATE transactions
SET
    account_id = sqlc.arg(account_id),
    amount = ROUND(sqlc.arg(amount), 6),
    kind = sqlc.arg(kind),
    category_id = sqlc.arg(category_id),
    counterparty_id = sqlc.arg(counterparty_id),
    description = sqlc.arg(description),
    note = sqlc.arg(note),
    to_account_id = sqlc.narg(to_account_id),
    to_amount = IIF(sqlc.narg(to_amount) > 0.0, ROUND(sqlc.narg(to_amount), 6), NULL)
WHERE recurrent_id = sqlc.arg(recurrent_id)
    AND fulfilled_at IS NULL
    AND NOT recurrent_overridden;

-- name: DeleteUnfulfilledOccurrencesFrom :exec
-- Deletes a recurrent tx's scheduled occurrences on or after from_date, the
-- ones edited on their own too unless keep_overridden.
DELETE FROM transactions
WHERE recurrent_id = sqlc.arg(recurrent_id)
    AND fulfilled_at IS NULL
    AND scheduled_to >= sqlc.arg(from_date)
    AND (NOT CAST(sqlc.arg(keep_overridden) AS BOOLEAN) OR NOT recurrent_overridden);

-- name: GetAutoFulfillDue :many
-- The scheduled occurrences, due by until, of the recurrent txs fulfilling
-- them by themselves.
SELECT t.id, t.account_id, t.kind, t.amount, t.to_account_id, t.to_amount, t.scheduled_to
FROM transactions AS t
INNER JOIN recurrent_txs AS rt ON t.recurrent_id = rt.id
WHERE rt.auto_fulfill
    AND rt.paused_at IS NULL
    AND t.fulfilled_at IS NULL
    AND t.scheduled_to < sqlc.arg(until);

-- name: FulfillTransactionOn :exec
-- Fulfills a transaction at a given time (an occurrence fulfilled by itself
-- or by the SMS it got merged with), flagged for review.
UPDATE transactions
SET
    fulfilled_at = sqlc.arg(fulfilled_at),
    needs_review = sqlc.arg(needs_review)
WHERE id = sqlc.arg(id);

-- name: GetRecurrentOccurrenceCandidates :many
-- Occurrences of recurrent txs out of account_id scheduled between since and
-- until that an SMS may be the payment of: scheduled ones, or fulfilled by
-- themselves (auto_fulfill) with no SMS (nor CSV row) yet.
SELECT sqlc.embed(t), cp.is_merchant AS counterparty_is_merchant
FROM transactions AS t
INNER JOIN recurrent_txs AS rt ON t.recurrent_id = rt.id
LEFT JOIN counterparties AS cp ON t.counterparty_id = cp.id
WHERE t.account_id = sqlc.arg(account_id)
    AND t.id != sqlc.arg(id)
    AND t.scheduled_to >= sqlc.arg(since)
    AND t.scheduled_to <= sqlc.arg(until)
    AND t.transfer_pair_id IS NULL
    AND t.reimburses_id IS NULL
    AND (
        t.fulfilled_at IS NULL
        OR (rt.auto_fulfill AND t.import_id IS NULL AND NOT EXISTS (SELECT 1 FROM sms_messages AS s WHERE s.transaction_id = t.id))
    )
ORDER BY t.scheduled_to;

-- name: GetUnlinkedSMSTransactions :many
-- Fulfilled SMS transactions out of account_id, between since and until, not
-- generated by a recurrent tx: maybe the payment of an occurrence about to be
-- written.
SELECT t.*
FROM transactions AS t
WHERE t.account_id = sqlc.arg(account_id)
    AND t.recurrent_id IS NULL
    AND t.fulfilled_at IS NOT NULL
    AND t.scheduled_to >= sqlc.arg(since)
    AND t.scheduled_to <= sqlc.arg(until)
    AND t.transfer_pair_id IS NULL
    AND t.reimburses_id IS NULL
    AND EXISTS (SELECT 1 FROM sms_messages AS s WHERE s.transaction_id = t.id)
ORDER BY t.scheduled_to;

-- name: SetTransactionRecurrent :exec
UPDATE transactions
SET recurrent_id = sqlc.arg(recurrent_id)
WHERE id = sqlc.arg(id);

-- name: MoveSMSMessagesTransaction :exec
UPDATE sms_messages
SET transaction_id = sqlc.arg(to_id)
WHERE transaction_id = sqlc.arg(from_id);


-- name: DeleteRecurrentTx :exec
DELETE FROM recurrent_txs
WHERE id = sqlc.arg(id);

-- name: DeleteScheduledTransactionsOfRecurrentTx :exec
DELETE FROM transactions AS tx
WHERE (
    tx.recurrent_id = sqlc.arg(recurrent_id)
    AND tx.fulfilled_at IS NULL
);

-- name: GetRecurrentTxsOccurrences :many
-- Dates of the transactions generated from recurrent txs, scheduled and
-- fulfilled ones.
SELECT
    tx.recurrent_id,
    tx.scheduled_to,
    tx.fulfilled_at
FROM transactions AS tx
WHERE tx.recurrent_id IS NOT NULL
ORDER BY tx.scheduled_to;

-- name: GetRecurrentTxOccurrences :many
SELECT
    tx.recurrent_id,
    tx.scheduled_to,
    tx.fulfilled_at
FROM transactions AS tx
WHERE tx.recurrent_id = sqlc.arg(recurrent_id)
ORDER BY tx.scheduled_to;

-- name: GetAutoFulfillRecurrentTxIDs :many
-- The recurrent txs fulfilling their occurrences by themselves, to tell their
-- transactions in the list.
SELECT id FROM recurrent_txs WHERE auto_fulfill;
