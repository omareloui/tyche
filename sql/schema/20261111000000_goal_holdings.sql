-- +goose Up
-- +goose StatementBegin
-- A goal holds money in every currency it was put in with: its own account
-- (goals.account_id, in the goal's currency) and one more hidden account per
-- other currency (a holding), so the money keeps its value in the account it
-- sits in and the goal is worth it at today's rates. goal_id ties every
-- account of a goal to it; the seeder fills it for the existing goals' own
-- accounts, moving the deposits made from other currencies to holdings. A
-- deleted goal's (soft deleted) accounts lose it.
ALTER TABLE accounts ADD COLUMN goal_id VARCHAR REFERENCES goals (id) ON DELETE SET NULL;
CREATE UNIQUE INDEX accounts_goal_currency ON accounts (goal_id, currency_id)
WHERE goal_id IS NOT NULL AND deleted_at IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX accounts_goal_currency;
ALTER TABLE accounts DROP COLUMN goal_id;
-- +goose StatementEnd
