-- +goose Up
-- +goose StatementBegin
CREATE TABLE currencies (
    id VARCHAR(16) UNIQUE NOT NULL PRIMARY KEY,
    enum INT UNIQUE NOT NULL,
    egp_fx_rate FLOAT DEFAULT 1 NOT NULL
);
CREATE TABLE transaction_kinds (
    id VARCHAR(16) UNIQUE NOT NULL PRIMARY KEY,
    kind VARCHAR(255) UNIQUE NOT NULL,
    increase_balance BOOLEAN DEFAULT FALSE
);
CREATE TABLE balances (
    id VARCHAR(16) UNIQUE NOT NULL PRIMARY KEY,
    amount FLOAT DEFAULT 0 NOT NULL,
    currency_id VARCHAR(16) UNIQUE NOT NULL,
    FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE
);

CREATE TABLE transactions (
    id VARCHAR(16) UNIQUE NOT NULL PRIMARY KEY,
    amount FLOAT DEFAULT 0 NOT NULL,
    currency_id VARCHAR(16) NOT NULL,
    kind_id VARCHAR(16) NOT NULL,
    description VARCHAR(225),
    fulfilled_on DATE,
    scheduled_to DATE NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW,
    FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE,
    FOREIGN KEY (kind_id) REFERENCES transaction_kinds (id)
);
CREATE TABLE frequent_txs (
    id VARCHAR(16) UNIQUE NOT NULL PRIMARY KEY,
    frequency_by INT NOT NULL,
    frequency INT NOT NULL,
    amount FLOAT DEFAULT 0 NOT NULL,
    currency_id VARCHAR(16) NOT NULL,
    kind_id VARCHAR(16) NOT NULL,
    description VARCHAR(225),
    starts_on TIMESTAMP NOT NULL,
    stops_on TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW,
    last_transaction_created_at TIMESTAMP NOT NULL DEFAULT NOW,
    FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE,
    FOREIGN KEY (kind_id) REFERENCES transaction_kinds (id)
);
CREATE TABLE frequent_txs_transactions (
    transaction_id VARCHAR(16) NOT NULL,
    frequency_id VARCHAR(16) NOT NULL,
    FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE CASCADE,
    FOREIGN KEY (frequency_id) REFERENCES frequent_txs (id) ON DELETE CASCADE
);
CREATE TABLE goals (
    id VARCHAR(16) UNIQUE NOT NULL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    egp_subtotal FLOAT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW
);
CREATE TABLE goals_items (
    goal_id VARCHAR(16) NOT NULL,
    item_id VARCHAR(16) NOT NULL,
    FOREIGN KEY (goal_id) REFERENCES goals (id) ON DELETE CASCADE,
    FOREIGN KEY (item_id) REFERENCES items (id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_goals_items
ON goals_items (goal_id, item_id);
CREATE TABLE items (
    id VARCHAR(16) UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    price FLOAT DEFAULT 0 NOT NULL,
    currency_id VARCHAR(16) NOT NULL,
    purchased_on DATE,
    FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE
);
CREATE TABLE account_kinds  ( 
  id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
  enum INT UNIQUE NOT NULL
);
CREATE TABLE accounts (
    id VARCHAR(16) UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR UNIQUE NOT NULL,
    kind_id VARCHAR NOT NULL,
    amount FLOAT NOT NULL,
    currency_id VARCHAR NOT NULL,
    card_number VARCHAR UNIQUE,
    iban VARCHAR UNIQUE,
    note VARCHAR,
    is_archived BOOL,
    exclude_from_balance BOOL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW,
    FOREIGN KEY (currency_id) REFERENCES currencies (id)
    FOREIGN KEY (kind_id) REFERENCES account_kinds (id)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE balances;
DROP TABLE currencies;
DROP TABLE transaction_kinds;
DROP TABLE transactions;
DROP TABLE goals;
DROP TABLE goals_items;
DROP INDEX idx_goals_items;
DROP TABLE items;
DROP TABLE accounts;
DROP TABLE account_kinds;
-- +goose StatementEnd
