-- +goose Up
-- +goose StatementBegin
CREATE TABLE currencies (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    enum INT UNIQUE NOT NULL,
    egp_fx_rate FLOAT DEFAULT 1 NOT NULL
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
    card_number VARCHAR UNIQUE,
    iban VARCHAR UNIQUE,
    note VARCHAR,
    is_default BOOL NOT NULL DEFAULT FALSE,
    is_archived BOOL NOT NULL DEFAULT FALSE,
    exclude_from_balance BOOL NOT NULL DEFAULT FALSE,
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
    fulfilled_at DATE,
    scheduled_to DATE NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE,
    FOREIGN KEY (to_account_id) REFERENCES accounts (id) ON DELETE CASCADE,
    FOREIGN KEY (counterparty_id) REFERENCES counterparties (id) ON DELETE SET NULL,
    FOREIGN KEY (kind_id) REFERENCES transaction_kinds (id)
);
CREATE TABLE frequent_txs (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    frequency_by INT NOT NULL,
    frequency INT NOT NULL,
    account_id VARCHAR NOT NULL,
    amount FLOAT DEFAULT 0 NOT NULL,
    kind_id VARCHAR NOT NULL,
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
    FOREIGN KEY (counterparty_id) REFERENCES counterparties (id) ON DELETE SET NULL,
    FOREIGN KEY (kind_id) REFERENCES transaction_kinds (id)
);
CREATE TABLE frequent_txs_transactions (
    transaction_id VARCHAR NOT NULL,
    frequency_id VARCHAR NOT NULL,
    FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE CASCADE,
    FOREIGN KEY (frequency_id) REFERENCES frequent_txs (id) ON DELETE CASCADE
);
CREATE TABLE goals (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    title VARCHAR NOT NULL,
    egp_subtotal FLOAT NOT NULL,
    fulfilled_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE goals_items (
    goal_id VARCHAR NOT NULL,
    item_id VARCHAR NOT NULL,
    FOREIGN KEY (goal_id) REFERENCES goals (id) ON DELETE CASCADE,
    FOREIGN KEY (item_id) REFERENCES items (id) ON DELETE CASCADE
);
CREATE TABLE goals_progresses (
    goal_id VARCHAR NOT NULL,
    transaction_id VARCHAR NOT NULL,
    FOREIGN KEY (goal_id) REFERENCES goals (id) ON DELETE CASCADE,
    FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE CASCADE
);
CREATE TABLE items (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR NOT NULL,
    price FLOAT DEFAULT 0 NOT NULL,
    currency_id VARCHAR NOT NULL,
    purchased_at DATE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE
);
CREATE TABLE categories (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR UNIQUE NOT NULL,
    budget_in_egp FLOAT DEFAULT 0 NOT NULL,
    kind_id VARCHAR NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (kind_id) REFERENCES transaction_kinds (id) ON DELETE CASCADE
);
CREATE TABLE counterparties (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR UNIQUE NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE currencies;
DROP TABLE transaction_kinds;
DROP TABLE transactions;
DROP TABLE goals;
DROP TABLE goals_items;
DROP TABLE goals_progresses;
DROP TABLE items;
DROP TABLE accounts;
DROP TABLE account_kinds;
DROP TABLE categories;
DROP TABLE counterparties;
-- +goose StatementEnd
