-- +goose Up
-- +goose StatementBegin
-- What an hour of work earns, to price things in work hours. NULL salary =
-- not set.
ALTER TABLE user ADD COLUMN monthly_salary FLOAT;
ALTER TABLE user ADD COLUMN salary_currency_id VARCHAR REFERENCES currencies (id);
ALTER TABLE user ADD COLUMN work_hours_per_week FLOAT NOT NULL DEFAULT 40;

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
    FOREIGN KEY (currency_id) REFERENCES currencies (id),
    FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE SET NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE wishlist_items;
ALTER TABLE user DROP COLUMN work_hours_per_week;
ALTER TABLE user DROP COLUMN salary_currency_id;
ALTER TABLE user DROP COLUMN monthly_salary;
-- +goose StatementEnd
