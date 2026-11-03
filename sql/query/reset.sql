-- name: ResetDeleteTransactions :exec
DELETE FROM transactions;

-- name: ResetZeroAccountBalances :exec
UPDATE accounts SET amount = 0;

-- name: ResetDeleteRecurrentTxs :exec
DELETE FROM recurrent_txs;

-- name: ResetDeleteSMSMessages :exec
DELETE FROM sms_messages;

-- name: ResetDeleteSMSTemplates :exec
DELETE FROM sms_templates;

-- name: ResetDeleteGoals :exec
DELETE FROM goals;

-- Goals' accounts, deleted goals' ones included. Their transactions are
-- reset too (ResetGoals requires it).
-- name: ResetDeleteGoalAccounts :exec
DELETE FROM accounts
WHERE kind_id IN (SELECT id FROM account_kinds WHERE enum = 6); -- models.AccKindGoal

-- name: ResetDeleteWishlistItems :exec
DELETE FROM wishlist_items;

-- name: ResetDeleteDevices :exec
DELETE FROM devices;

-- category_id has no foreign key, so the references are cleared by hand.
-- name: ResetClearTxCategories :exec
UPDATE transactions SET category_id = NULL;

-- name: ResetClearRecurrentTxCategories :exec
UPDATE recurrent_txs SET category_id = NULL;

-- name: ResetDeleteCategories :exec
DELETE FROM categories;

-- accounts.counterparty_id has no ON DELETE, so it's cleared first.
-- name: ResetClearAccountCounterparties :exec
UPDATE accounts SET counterparty_id = NULL;

-- name: ResetDeleteCounterparties :exec
DELETE FROM counterparties;

-- Gam'yaas go with their accounts (ON DELETE CASCADE).
-- name: ResetDeleteAccounts :exec
DELETE FROM accounts;

-- name: ResetUserSettings :exec
UPDATE user
SET
    monthly_salary = NULL,
    salary_currency_id = NULL,
    work_hours_per_week = 40,
    timezone = 'Africa/Cairo',
    default_account_id = NULL,
    default_currency_id = COALESCE(
        (SELECT c.id FROM currencies AS c WHERE c.string = sqlc.arg(default_currency)),
        default_currency_id
    );

-- name: ResetCurrenciesShown :exec
UPDATE currencies
SET is_shown = (string IN (sqlc.slice(shown)));

-- name: ResetSeededAt :exec
UPDATE user SET seeded_at = NULL;
