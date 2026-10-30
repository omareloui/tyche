-- +goose Up
-- +goose StatementBegin
-- Incomes from merchants are refunds (kind 5), as merchant spending already
-- treated them.
UPDATE transactions
SET kind = 5
WHERE
    kind = 2
    AND counterparty_id IN (SELECT id FROM counterparties WHERE is_merchant IS TRUE);
-- Refunds use expense categories, give the uncategorized ones their merchant's
-- default category if it allows expenses.
UPDATE transactions
SET category_id = (
    SELECT cp.default_category_id
    FROM counterparties AS cp
    JOIN categories AS c ON c.id = cp.default_category_id
    WHERE
        cp.id = transactions.counterparty_id
        AND EXISTS (SELECT 1 FROM JSON_EACH(c.kinds) WHERE value = 1)
)
WHERE kind = 5 AND category_id IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE transactions SET kind = 2 WHERE kind = 5;
UPDATE recurrent_txs SET kind = 2 WHERE kind = 5;
-- +goose StatementEnd
