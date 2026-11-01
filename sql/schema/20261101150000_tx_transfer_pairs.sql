-- +goose Up
-- +goose StatementBegin
-- The two legs (an expense and an income) of a transfer between the user's
-- own accounts that came as two SMS, each pointing at the other. Both are
-- left out of every analysis; the income is shown nested under the expense.
ALTER TABLE transactions ADD COLUMN transfer_pair_id VARCHAR REFERENCES transactions (id) ON DELETE SET NULL;
CREATE INDEX transactions_transfer_pair_id ON transactions (transfer_pair_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX transactions_transfer_pair_id;
ALTER TABLE transactions DROP COLUMN transfer_pair_id;
-- +goose StatementEnd
