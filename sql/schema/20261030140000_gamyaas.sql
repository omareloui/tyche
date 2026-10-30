-- +goose Up
-- +goose StatementBegin
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
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE gamyaas;
-- +goose StatementEnd
