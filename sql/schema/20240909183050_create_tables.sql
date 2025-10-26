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
CREATE TABLE transaction_kinds (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    enum INT UNIQUE NOT NULL,
    increase_balance BOOLEAN
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
    FOREIGN KEY (currency_id) REFERENCES currencies (id),
    FOREIGN KEY (counterparty_id) REFERENCES counterparties (id),
    FOREIGN KEY (kind_id) REFERENCES account_kinds (id)
);
CREATE TABLE transactions (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    account_id VARCHAR NOT NULL,
    amount FLOAT DEFAULT 0 NOT NULL,
    kind_id VARCHAR NOT NULL,
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
    FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE,
    FOREIGN KEY (to_account_id) REFERENCES accounts (id) ON DELETE CASCADE,
    FOREIGN KEY (counterparty_id) REFERENCES counterparties (id) ON DELETE SET NULL,
    FOREIGN KEY (kind_id) REFERENCES transaction_kinds (id),
    FOREIGN KEY (recurrent_id) REFERENCES recurrent_txs (id) ON DELETE SET NULL,
    FOREIGN KEY (goal_id) REFERENCES goals (id) ON DELETE SET NULL
);
CREATE TABLE recurrent_txs (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    every INT NOT NULL,
    unit INT NOT NULL,
    account_id VARCHAR NOT NULL,
    amount FLOAT DEFAULT 0 NOT NULL,
    kind_id VARCHAR NOT NULL,
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
    FOREIGN KEY (kind_id) REFERENCES transaction_kinds (id),
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
    kind_id VARCHAR NOT NULL,
    color VARCHAR NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (kind_id) REFERENCES transaction_kinds (id) ON DELETE CASCADE,
    FOREIGN KEY (budget_currency_id) REFERENCES currencies (id) ON DELETE CASCADE
);
CREATE TABLE counterparties (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR UNIQUE NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

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
    rt.kind_id AS recurrent_tx_kind_id,
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

    -- Kind
    k.enum AS kind_enum,
    k.increase_balance AS kind_increase_balance,

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
    cat.kind_id AS category_kind_id,
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
LEFT JOIN transaction_kinds AS k ON rt.kind_id = k.id
LEFT JOIN categories AS cat ON rt.category_id = cat.id
LEFT JOIN goals AS g ON rt.goal_id = g.id
LEFT JOIN currencies AS gc ON g.currency_id = gc.id;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE user;
DROP TABLE currencies;
DROP TABLE transaction_kinds;
DROP TABLE transactions;
DROP TABLE recurrent_txs;
DROP TABLE goals;
DROP TABLE accounts;
DROP TABLE account_kinds;
DROP TABLE categories;
DROP TABLE counterparties;

-- Views
DROP VIEW IF EXISTS recurrent_transactions_detail;
-- +goose StatementEnd
