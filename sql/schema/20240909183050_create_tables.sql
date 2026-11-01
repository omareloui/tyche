-- +goose Up
-- +goose StatementBegin
CREATE TABLE user (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    oauth_id VARCHAR UNIQUE NOT NULL,
    oauth_provider INT NOT NULL,
    email VARCHAR NOT NULL,
    verified_email BOOLEAN NOT NULL DEFAULT FALSE,
    first_name VARCHAR NOT NULL,
    last_name VARCHAR NOT NULL,
    picture VARCHAR,
    default_currency_id VARCHAR NOT NULL,
    default_account_id VARCHAR,
    start_of_period INT NOT NULL DEFAULT 1 CHECK (start_of_period BETWEEN 1 AND 31),
    seeded_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- What an hour of work earns, to price things in work hours. NULL salary =
    -- not set.
    monthly_salary FLOAT,
    salary_currency_id VARCHAR REFERENCES currencies (id),
    work_hours_per_week FLOAT NOT NULL DEFAULT 40,
    -- The time zone of the dates and times in the user's bank sms.
    timezone VARCHAR NOT NULL DEFAULT 'Africa/Cairo',
    -- When the user was asked whether to encrypt their database (with a
    -- password), asked once on signing in while NULL.
    encryption_prompted_at TIMESTAMP,
    -- The profile picture, downloaded on signing in and served from /avatar (the
    -- provider's url is rate limited). picture_hash versions its url.
    picture_data BLOB,
    picture_type TEXT,
    picture_hash TEXT,
    FOREIGN KEY (default_account_id) REFERENCES accounts (id) ON DELETE SET NULL,
    FOREIGN KEY (default_currency_id) REFERENCES currencies (id)
);
CREATE TABLE currencies (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    string VARCHAR UNIQUE NOT NULL,
    symbol VARCHAR UNIQUE NOT NULL,
    kind VARCHAR NOT NULL,
    iban_prefix VARCHAR,
    is_shown BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE TABLE account_kinds (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    enum INT UNIQUE NOT NULL
);
CREATE TABLE accounts (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR UNIQUE NOT NULL,
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
    FOREIGN KEY (currency_id) REFERENCES currencies (id),
    FOREIGN KEY (counterparty_id) REFERENCES counterparties (id),
    FOREIGN KEY (kind_id) REFERENCES account_kinds (id)
);
CREATE TABLE transactions (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    account_id VARCHAR NOT NULL,
    amount FLOAT DEFAULT 0 NOT NULL,
    kind INT NOT NULL,
    category_id VARCHAR,
    counterparty_id VARCHAR,
    description VARCHAR,
    to_account_id VARCHAR,
    to_amount FLOAT,
    goal_id VARCHAR,
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
    FOREIGN KEY (recurrent_id) REFERENCES recurrent_txs (id) ON DELETE SET NULL,
    FOREIGN KEY (goal_id) REFERENCES goals (id) ON DELETE SET NULL
);
CREATE TABLE recurrent_txs (
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
    goal_id VARCHAR,
    starts_at TIMESTAMP NOT NULL,
    stops_at TIMESTAMP,
    last_transaction_scheduled_to TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE,
    FOREIGN KEY (to_account_id) REFERENCES accounts (id) ON DELETE CASCADE,
    FOREIGN KEY (counterparty_id) REFERENCES counterparties (id) ON DELETE SET NULL,
    FOREIGN KEY (goal_id) REFERENCES goals (id) ON DELETE SET NULL
);
CREATE TABLE goals (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR NOT NULL,
    price FLOAT DEFAULT 0 NOT NULL,
    currency_id VARCHAR NOT NULL,
    fulfilled_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE
);
CREATE TABLE categories (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR UNIQUE NOT NULL,
    budget_amount FLOAT,
    budget_currency_id VARCHAR,
    kinds JSON NOT NULL,
    color VARCHAR NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (budget_currency_id) REFERENCES currencies (id) ON DELETE CASCADE
);
CREATE TABLE counterparties (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR UNIQUE NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    aliases JSON NOT NULL DEFAULT '[]',
    -- NULL means the icon was never detected, '' means no icon.
    icon VARCHAR,
    -- The brand color of the icon, or a random one for the letter placeholder.
    color VARCHAR,
    -- Only merchants get their brand icon auto-detected.
    is_merchant BOOLEAN NOT NULL DEFAULT FALSE,
    -- Used for new transactions with the counterparty and no category.
    default_category_id VARCHAR REFERENCES categories (id) ON DELETE SET NULL
);
CREATE TABLE devices (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR NOT NULL,
    last_seen_at TIMESTAMP,
    revoked_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Stable per app install id, signing in again from the same install reuses
    -- the device row.
    install_id VARCHAR,
    -- Id of the device's current token, rotated on signing in again so older
    -- tokens stop working. NULL for devices registered before this.
    token_id VARCHAR
);
CREATE TABLE sms_messages (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    sender VARCHAR NOT NULL,
    body VARCHAR NOT NULL,
    received_at TIMESTAMP NOT NULL,
    hash VARCHAR UNIQUE NOT NULL,
    status VARCHAR NOT NULL,
    error VARCHAR,
    transaction_id VARCHAR,
    device_id VARCHAR,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- The template that read it, no foreign key as it can be a shared one.
    template_id VARCHAR,
    FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE SET NULL,
    FOREIGN KEY (device_id) REFERENCES devices (id) ON DELETE SET NULL
);
-- The user's own templates reading bank sms, tried before the shared ones.
CREATE TABLE sms_templates (
    id VARCHAR NOT NULL PRIMARY KEY,
    name VARCHAR NOT NULL,
    bank VARCHAR NOT NULL DEFAULT '',
    senders JSON NOT NULL DEFAULT '[]',
    action VARCHAR NOT NULL,
    tx_kind INTEGER,
    body VARCHAR NOT NULL,
    fixed JSON NOT NULL DEFAULT '{}',
    sample VARCHAR NOT NULL DEFAULT '',
    priority INTEGER NOT NULL DEFAULT 0,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- A gam'yaa (rotating savings group) held as an account of kind 5: its
-- installments are transfers into it and its payouts transfers out of it, so
-- its balance is what's paid in (positive) or still owed (negative).
CREATE TABLE gamyaas (
    account_id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    -- A full share's monthly installment.
    installment FLOAT NOT NULL,
    -- Members, which is also its number of months.
    members INT NOT NULL,
    shares FLOAT NOT NULL DEFAULT 1,
    -- The 1-based months the pot is received in, e.g. [3] or [2, 9].
    turns JSON NOT NULL,
    starts_on DATE NOT NULL,
    pay_from_account_id VARCHAR,
    recurrent_tx_id VARCHAR,
    FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE,
    FOREIGN KEY (pay_from_account_id) REFERENCES accounts (id) ON DELETE SET NULL,
    FOREIGN KEY (recurrent_tx_id) REFERENCES recurrent_txs (id) ON DELETE SET NULL
);
-- Things to buy after a cooling-off period. status: 0 waiting, 1 bought,
-- 2 abandoned. installments = 0 means paying cash.
CREATE TABLE wishlist_items (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR NOT NULL,
    price FLOAT NOT NULL,
    currency_id VARCHAR NOT NULL,
    installments INT NOT NULL DEFAULT 0,
    installment_amount FLOAT NOT NULL DEFAULT 0,
    url VARCHAR NOT NULL DEFAULT '',
    note VARCHAR NOT NULL DEFAULT '',
    cooling_days INT NOT NULL CHECK (cooling_days IN (7, 30)),
    status INT NOT NULL DEFAULT 0,
    decided_at TIMESTAMP,
    transaction_id VARCHAR,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- How a wishlist item is paid in installments (models.InstallmentPlan:
    -- strategy, rate, down payment, fees, ...), NULL when paid cash or for items
    -- from before plans, which only have installments and installment_amount.
    installment_plan JSON,
    FOREIGN KEY (currency_id) REFERENCES currencies (id),
    FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE SET NULL
);

-------------------------------------------
----------------- INDEXES -----------------
-------------------------------------------

CREATE UNIQUE INDEX devices_install_id_idx ON devices (install_id);
CREATE INDEX transactions_reimburses_id ON transactions (reimburses_id);
CREATE INDEX transactions_transfer_pair_id ON transactions (transfer_pair_id);
CREATE INDEX transactions_scheduled_to ON transactions (scheduled_to, created_at);
-- Transaction lists look up each row's sms (body, from-sms filter).
CREATE INDEX sms_messages_transaction_id ON sms_messages (transaction_id);

-------------------------------------------
------------------ VIEWS ------------------
-------------------------------------------

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
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Views
DROP VIEW IF EXISTS recurrent_txs_details;

DROP TABLE wishlist_items;
DROP TABLE gamyaas;
DROP TABLE sms_templates;
DROP TABLE sms_messages;
DROP TABLE devices;
DROP TABLE user;
DROP TABLE currencies;
DROP TABLE transactions;
DROP TABLE recurrent_txs;
DROP TABLE goals;
DROP TABLE accounts;
DROP TABLE account_kinds;
DROP TABLE categories;
DROP TABLE counterparties;
-- +goose StatementEnd
