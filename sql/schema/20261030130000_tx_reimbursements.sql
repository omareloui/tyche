-- +goose Up
-- +goose StatementBegin
-- An income paying back (part of) an expense, e.g. a friend's share of a
-- bill. The expense is analysed net of its reimbursements and the
-- reimbursements themselves count as 0.
ALTER TABLE transactions ADD COLUMN reimburses_id VARCHAR REFERENCES transactions (id) ON DELETE SET NULL;
CREATE INDEX transactions_reimburses_id ON transactions (reimburses_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX transactions_reimburses_id;
ALTER TABLE transactions DROP COLUMN reimburses_id;
-- +goose StatementEnd
