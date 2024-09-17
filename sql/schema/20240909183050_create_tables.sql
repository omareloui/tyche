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
CREATE TABLE frequent_transactions (
    id VARCHAR(16) UNIQUE NOT NULL PRIMARY KEY,
    frequency_by INT NOT NULL,
    frequency INT NOT NULL,
    amount FLOAT DEFAULT 0 NOT NULL,
    currency_id VARCHAR(16) NOT NULL,
    kind_id VARCHAR(16) NOT NULL,
    description VARCHAR(225),
    stops_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW,
    last_transaction_created_at TIMESTAMP NOT NULL DEFAULT NOW,
    FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE,
    FOREIGN KEY (kind_id) REFERENCES transaction_kinds (id)
);
-- TODO: to be able to delete not finished transactions
CREATE TABLE frequent_transactions_transactions (
    transaction_id VARCHAR(16) NOT NULL,
    frequency_id VARCHAR(16) NOT NULL,
    FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE CASCADE,
    FOREIGN KEY (frequency_id) REFERENCES frequent_transactions (id) ON DELETE CASCADE
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
CREATE TABLE items (
    id VARCHAR(16) UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    price FLOAT DEFAULT 0 NOT NULL,
    currency_id VARCHAR(16) NOT NULL,
    purchased_on DATE,
    FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE
);
-- CREATE TABLE goal_plan_entries (
--     id VARCHAR(16) UNIQUE NOT NULL PRIMARY KEY,
--     amount FLOAT DEFAULT 0 NOT NULL,
--     currency_id VARCHAR(16) NOT NULL,
--     kind_id VARCHAR(16) NOT NULL,
--     goal_id VARCHAR(16) NOT NULL,
--     FOREIGN KEY (currency_id) REFERENCES currencies (id),
--     FOREIGN KEY (goal_id) REFERENCES goals (id) ON DELETE CASCADE,
--     FOREIGN KEY (kind_id) REFERENCES transaction_kinds (id)
-- );

CREATE UNIQUE INDEX idx_goals_items
ON goals_items (goal_id, item_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE balances;
DROP TABLE currencies;
DROP TABLE transaction_kinds;
DROP TABLE transactions;
DROP TABLE goals;
DROP TABLE goals_items;
DROP TABLE items;
DROP INDEX idx_goals_items;
-- DROP TABLE goal_plan_entries;
-- +goose StatementEnd
