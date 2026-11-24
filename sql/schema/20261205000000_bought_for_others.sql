-- +goose Up
-- +goose StatementBegin
-- An expense the user paid for someone else (a gift, or to be paid back by
-- incomes reimbursing it): out of every analysis. for_others is the flag,
-- for_person_id who it was for (deleting the person keeps the flag).
ALTER TABLE transactions ADD COLUMN for_others BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE transactions ADD COLUMN for_person_id VARCHAR REFERENCES counterparties (id) ON DELETE SET NULL;
CREATE INDEX transactions_for_person_id ON transactions (for_person_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX transactions_for_person_id;
ALTER TABLE transactions DROP COLUMN for_person_id;
ALTER TABLE transactions DROP COLUMN for_others;
-- +goose StatementEnd
