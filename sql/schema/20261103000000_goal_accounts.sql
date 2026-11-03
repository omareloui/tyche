-- +goose NO TRANSACTION
-- Goals get their own (hidden) account: saving for one is a transfer to it,
-- buying it an expense from it. The old goal links of transactions and
-- recurrent transactions go. Accounts are soft deleted (deleted_at), so their
-- names only have to be unique among the live ones.
--
-- Dropping a column with a foreign key needs a table rebuild, and DROP TABLE
-- with the foreign keys on would fire the ON DELETE actions of the tables
-- pointing at it (e.g. clear transactions.recurrent_id), so they're off for
-- the rebuild (a no-op inside a transaction, hence NO TRANSACTION).

-- +goose Up
PRAGMA foreign_keys = OFF;

-- +goose StatementBegin
BEGIN;

DROP VIEW recurrent_txs_details;

-- The goal's account, filled by the seeder (ids are ULIDs made in Go).
ALTER TABLE goals ADD COLUMN account_id VARCHAR REFERENCES accounts (id);

CREATE TABLE accounts_new (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR NOT NULL,
    kind_id VARCHAR NOT NULL,
    amount FLOAT NOT NULL,
    currency_id VARCHAR NOT NULL,
    counterparty_id VARCHAR,
    card_number VARCHAR,
    card_limit FLOAT DEFAULT 0.0 NOT NULL,
    name_on_card VARCHAR,
    iban VARCHAR,
    note VARCHAR,
    is_archived BOOL NOT NULL DEFAULT FALSE,
    exclude_from_balance BOOL NOT NULL DEFAULT FALSE,
    expiration_date DATE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- The ways bank sms refer to the account (account number, card, ...).
    sms_refs JSON NOT NULL DEFAULT '[]',
    -- Deleted accounts are kept for the history of their transactions, hidden
    -- everywhere else.
    deleted_at TIMESTAMP,
    FOREIGN KEY (currency_id) REFERENCES currencies (id),
    FOREIGN KEY (counterparty_id) REFERENCES counterparties (id),
    FOREIGN KEY (kind_id) REFERENCES account_kinds (id)
);
INSERT INTO accounts_new (
    id, name, kind_id, amount, currency_id, counterparty_id, card_number,
    card_limit, name_on_card, iban, note, is_archived, exclude_from_balance,
    expiration_date, created_at, sms_refs
)
SELECT
    id, name, kind_id, amount, currency_id, counterparty_id, card_number,
    card_limit, name_on_card, iban, note, is_archived, exclude_from_balance,
    expiration_date, created_at, sms_refs
FROM accounts;
DROP TABLE accounts;
ALTER TABLE accounts_new RENAME TO accounts;
CREATE UNIQUE INDEX accounts_live_name ON accounts (name) WHERE deleted_at IS NULL;

CREATE TABLE recurrent_txs_new (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    every INT NOT NULL,
    unit INT NOT NULL,
    account_id VARCHAR NOT NULL,
    amount FLOAT DEFAULT 0 NOT NULL,
    kind INT NOT NULL,
    category_id VARCHAR,
    counterparty_id VARCHAR,
    description VARCHAR,
    to_account_id VARCHAR,
    to_amount FLOAT,
    starts_at TIMESTAMP NOT NULL,
    stops_at TIMESTAMP,
    last_transaction_scheduled_to TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE,
    FOREIGN KEY (to_account_id) REFERENCES accounts (id) ON DELETE CASCADE,
    FOREIGN KEY (counterparty_id) REFERENCES counterparties (id) ON DELETE SET NULL
);
INSERT INTO recurrent_txs_new (
    id, every, unit, account_id, amount, kind, category_id, counterparty_id,
    description, to_account_id, to_amount, starts_at, stops_at,
    last_transaction_scheduled_to, created_at
)
SELECT
    id, every, unit, account_id, amount, kind, category_id, counterparty_id,
    description, to_account_id, to_amount, starts_at, stops_at,
    last_transaction_scheduled_to, created_at
FROM recurrent_txs;
DROP TABLE recurrent_txs;
ALTER TABLE recurrent_txs_new RENAME TO recurrent_txs;

CREATE TABLE transactions_new (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    account_id VARCHAR NOT NULL,
    amount FLOAT DEFAULT 0 NOT NULL,
    kind INT NOT NULL,
    category_id VARCHAR,
    counterparty_id VARCHAR,
    description VARCHAR,
    to_account_id VARCHAR,
    to_amount FLOAT,
    recurrent_id VARCHAR,
    fulfilled_at DATE,
    scheduled_to DATE NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    needs_review BOOLEAN NOT NULL DEFAULT FALSE,
    -- An income paying back (part of) an expense, e.g. a friend's share of a
    -- bill. The expense is analysed net of its reimbursements and the
    -- reimbursements themselves count as 0.
    reimburses_id VARCHAR REFERENCES transactions (id) ON DELETE SET NULL,
    -- The two legs (an expense and an income) of a transfer between the user's
    -- own accounts that came as two SMS, each pointing at the other. Both are
    -- left out of every analysis; the income is shown nested under the expense.
    transfer_pair_id VARCHAR REFERENCES transactions (id) ON DELETE SET NULL,
    FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE,
    FOREIGN KEY (to_account_id) REFERENCES accounts (id) ON DELETE CASCADE,
    FOREIGN KEY (counterparty_id) REFERENCES counterparties (id) ON DELETE SET NULL,
    FOREIGN KEY (recurrent_id) REFERENCES recurrent_txs (id) ON DELETE SET NULL
);
INSERT INTO transactions_new (
    id, account_id, amount, kind, category_id, counterparty_id, description,
    to_account_id, to_amount, recurrent_id, fulfilled_at, scheduled_to,
    created_at, needs_review, reimburses_id, transfer_pair_id
)
SELECT
    id, account_id, amount, kind, category_id, counterparty_id, description,
    to_account_id, to_amount, recurrent_id, fulfilled_at, scheduled_to,
    created_at, needs_review, reimburses_id, transfer_pair_id
FROM transactions;
DROP TABLE transactions;
ALTER TABLE transactions_new RENAME TO transactions;
CREATE INDEX transactions_reimburses_id ON transactions (reimburses_id);
CREATE INDEX transactions_transfer_pair_id ON transactions (transfer_pair_id);
CREATE INDEX transactions_scheduled_to ON transactions (scheduled_to, created_at);

CREATE VIEW recurrent_txs_details AS
SELECT
    rt.id AS recurrent_tx_id,
    rt.every AS recurrent_tx_every,
    rt.unit AS recurrent_tx_unit,
    rt.account_id AS recurrent_tx_account_id,
    rt.amount AS recurrent_tx_amount,
    rt.kind AS recurrent_tx_kind,
    rt.category_id AS recurrent_tx_category_id,
    rt.counterparty_id AS recurrent_tx_counterparty_id,
    rt.description AS recurrent_tx_description,
    rt.to_account_id AS recurrent_tx_to_account_id,
    rt.to_amount AS recurrent_tx_to_amount,
    rt.starts_at AS recurrent_tx_starts_at,
    rt.stops_at AS recurrent_tx_stops_at,
    rt.last_transaction_scheduled_to AS recurrent_tx_last_transaction_scheduled_to,
    rt.created_at AS recurrent_tx_created_at,

    -- Account details
    a.name AS account_name,
    a.kind_id AS account_kind_id,
    a.amount AS account_amount,
    a.currency_id AS account_currency_id,
    a.counterparty_id AS account_counterparty_id,
    a.card_number AS account_card_number,
    a.card_limit AS account_card_limit,
    a.name_on_card AS account_name_on_card,
    a.iban AS account_iban,
    a.note AS account_note,
    a.is_archived AS account_is_archived,
    a.exclude_from_balance AS account_exclude_from_balance,
    a.expiration_date AS account_expiration_date,
    a.created_at AS account_created_at,

    -- Account Currency
    c.string AS account_currency_string,
    c.symbol AS account_currency_symbol,

    -- Counterparty
    cp.name AS counterparty_name,

    -- To Account details
    ta.name AS to_account_name,
    ta.kind_id AS to_account_kind_id,
    ta.amount AS to_account_amount,
    ta.currency_id AS to_account_currency_id,
    ta.counterparty_id AS to_account_counterparty_id,
    ta.card_number AS to_account_card_number,
    ta.card_limit AS to_account_card_limit,
    ta.name_on_card AS to_account_name_on_card,
    ta.iban AS to_account_iban,
    ta.note AS to_account_note,
    ta.is_archived AS to_account_is_archived,
    ta.exclude_from_balance AS to_account_exclude_from_balance,
    ta.expiration_date AS to_account_expiration_date,
    ta.created_at AS to_account_created_at,

    -- To Account Currency
    tc.string AS to_account_currency_string,
    tc.symbol AS to_account_currency_symbol,

    -- Category
    cat.name AS category_name,
    cat.budget_amount AS category_budget_amount,
    cat.budget_currency_id AS category_budget_currency_id,
    cat.kinds AS category_kinds,
    cat.created_at AS category_created_at

FROM recurrent_txs AS rt
LEFT JOIN counterparties AS cp ON rt.counterparty_id = cp.id
LEFT JOIN accounts AS a ON rt.account_id = a.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN accounts AS ta ON rt.to_account_id = ta.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN categories AS cat ON rt.category_id = cat.id;

COMMIT;
-- +goose StatementEnd

PRAGMA foreign_keys = ON;

-- +goose Down
-- Only the dropped columns come back (empty); the soft deleted accounts are
-- deleted for real and account names are unique again.
PRAGMA foreign_keys = OFF;

-- +goose StatementBegin
BEGIN;

DROP VIEW recurrent_txs_details;

DELETE FROM transactions WHERE account_id IN (SELECT id FROM accounts WHERE deleted_at IS NOT NULL)
    OR to_account_id IN (SELECT id FROM accounts WHERE deleted_at IS NOT NULL);
DELETE FROM recurrent_txs WHERE account_id IN (SELECT id FROM accounts WHERE deleted_at IS NOT NULL)
    OR to_account_id IN (SELECT id FROM accounts WHERE deleted_at IS NOT NULL);
DELETE FROM accounts WHERE deleted_at IS NOT NULL;
DROP INDEX accounts_live_name;
CREATE UNIQUE INDEX accounts_name ON accounts (name);
ALTER TABLE accounts DROP COLUMN deleted_at;

CREATE TABLE goals_new (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR NOT NULL,
    price FLOAT DEFAULT 0 NOT NULL,
    currency_id VARCHAR NOT NULL,
    fulfilled_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE
);
INSERT INTO goals_new (id, name, price, currency_id, fulfilled_at, created_at)
SELECT id, name, price, currency_id, fulfilled_at, created_at FROM goals;
DROP TABLE goals;
ALTER TABLE goals_new RENAME TO goals;

ALTER TABLE transactions ADD COLUMN goal_id VARCHAR REFERENCES goals (id) ON DELETE SET NULL;
ALTER TABLE recurrent_txs ADD COLUMN goal_id VARCHAR REFERENCES goals (id) ON DELETE SET NULL;

CREATE VIEW recurrent_txs_details AS
SELECT
    rt.id AS recurrent_tx_id,
    rt.every AS recurrent_tx_every,
    rt.unit AS recurrent_tx_unit,
    rt.account_id AS recurrent_tx_account_id,
    rt.amount AS recurrent_tx_amount,
    rt.kind AS recurrent_tx_kind,
    rt.category_id AS recurrent_tx_category_id,
    rt.counterparty_id AS recurrent_tx_counterparty_id,
    rt.description AS recurrent_tx_description,
    rt.to_account_id AS recurrent_tx_to_account_id,
    rt.to_amount AS recurrent_tx_to_amount,
    rt.goal_id AS recurrent_tx_goal_id,
    rt.starts_at AS recurrent_tx_starts_at,
    rt.stops_at AS recurrent_tx_stops_at,
    rt.last_transaction_scheduled_to AS recurrent_tx_last_transaction_scheduled_to,
    rt.created_at AS recurrent_tx_created_at,

    -- Account details
    a.name AS account_name,
    a.kind_id AS account_kind_id,
    a.amount AS account_amount,
    a.currency_id AS account_currency_id,
    a.counterparty_id AS account_counterparty_id,
    a.card_number AS account_card_number,
    a.card_limit AS account_card_limit,
    a.name_on_card AS account_name_on_card,
    a.iban AS account_iban,
    a.note AS account_note,
    a.is_archived AS account_is_archived,
    a.exclude_from_balance AS account_exclude_from_balance,
    a.expiration_date AS account_expiration_date,
    a.created_at AS account_created_at,

    -- Account Currency
    c.string AS account_currency_string,
    c.symbol AS account_currency_symbol,

    -- Counterparty
    cp.name AS counterparty_name,

    -- To Account details
    ta.name AS to_account_name,
    ta.kind_id AS to_account_kind_id,
    ta.amount AS to_account_amount,
    ta.currency_id AS to_account_currency_id,
    ta.counterparty_id AS to_account_counterparty_id,
    ta.card_number AS to_account_card_number,
    ta.card_limit AS to_account_card_limit,
    ta.name_on_card AS to_account_name_on_card,
    ta.iban AS to_account_iban,
    ta.note AS to_account_note,
    ta.is_archived AS to_account_is_archived,
    ta.exclude_from_balance AS to_account_exclude_from_balance,
    ta.expiration_date AS to_account_expiration_date,
    ta.created_at AS to_account_created_at,

    -- To Account Currency
    tc.string AS to_account_currency_string,
    tc.symbol AS to_account_currency_symbol,

    -- Category
    cat.name AS category_name,
    cat.budget_amount AS category_budget_amount,
    cat.budget_currency_id AS category_budget_currency_id,
    cat.kinds AS category_kinds,
    cat.created_at AS category_created_at,

    -- Goal
    g.name AS goal_name,
    g.price AS goal_price,
    g.currency_id AS goal_currency_id,
    g.fulfilled_at AS goal_fulfilled_at,
    gc.string AS goal_currency_string,
    gc.symbol AS goal_currency_symbol

FROM recurrent_txs AS rt
LEFT JOIN counterparties AS cp ON rt.counterparty_id = cp.id
LEFT JOIN accounts AS a ON rt.account_id = a.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN accounts AS ta ON rt.to_account_id = ta.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN categories AS cat ON rt.category_id = cat.id
LEFT JOIN goals AS g ON rt.goal_id = g.id
LEFT JOIN currencies AS gc ON g.currency_id = gc.id;

COMMIT;
-- +goose StatementEnd

PRAGMA foreign_keys = ON;
