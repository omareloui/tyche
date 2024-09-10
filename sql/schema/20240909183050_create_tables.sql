-- +goose Up
-- +goose StatementBegin
CREATE TABLE currencies (
    id VARCHAR(16) UNIQUE NOT NULL PRIMARY KEY,
    currency VARCHAR(3) UNIQUE NOT NULL
);
CREATE TABLE transaction_kinds (
    id VARCHAR(16) UNIQUE NOT NULL PRIMARY KEY,
    kind VARCHAR(255) UNIQUE NOT NULL,
    increase_balance BOOLEAN DEFAULT FALSE
);
CREATE TABLE balances (
    id VARCHAR(16) UNIQUE NOT NULL PRIMARY KEY,
    amount INT DEFAULT 0 NOT NULL,
    currency_id VARCHAR(16) UNIQUE NOT NULL,
    FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE
);

CREATE TABLE transactions (
    id VARCHAR(16) UNIQUE NOT NULL PRIMARY KEY,
    amount INT DEFAULT 0 NOT NULL,
    currency_id VARCHAR(16) NOT NULL,
    transaction_kind_id VARCHAR(16) NOT NULL,
    can_repeat BOOLEAN DEFAULT FALSE NOT NULL,
    fulfilled_on DATE,
    scheduled_to DATE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW,
    FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE,
    FOREIGN KEY (transaction_kind_id) REFERENCES transaction_kinds (id)
);
CREATE TABLE goals (
    id VARCHAR(16) UNIQUE NOT NULL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    is_active BOOLEAN DEFAULT FALSE NOT NULL,
    achieved_on DATE,
    started_on DATE,
    sum_total_egp INT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW
);
CREATE TABLE goal_items (
    id VARCHAR(16) UNIQUE NOT NULL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    amount INT DEFAULT 0 NOT NULL,
    currency_id VARCHAR(16) NOT NULL,
    goal_id VARCHAR(16) NOT NULL,
    FOREIGN KEY (goal_id) REFERENCES goals (id) ON DELETE CASCADE,
    FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE
);
CREATE TABLE goal_plan_entries (
    id VARCHAR(16) UNIQUE NOT NULL PRIMARY KEY,
    amount INT DEFAULT 0 NOT NULL,
    currency_id VARCHAR(16) NOT NULL,
    transaction_kind_id VARCHAR(16) NOT NULL,
    goal_id VARCHAR(16) NOT NULL,
    FOREIGN KEY (currency_id) REFERENCES currencies (id),
    FOREIGN KEY (goal_id) REFERENCES goals (id) ON DELETE CASCADE,
    FOREIGN KEY (transaction_kind_id) REFERENCES transaction_kinds (id)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE balances;
DROP TABLE currencies;
DROP TABLE transaction_kinds;
DROP TABLE transactions;
DROP TABLE goals;
DROP TABLE goal_items;
DROP TABLE goal_plan_entries;
-- +goose StatementEnd
