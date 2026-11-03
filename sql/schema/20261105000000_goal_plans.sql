-- +goose Up
-- +goose StatementBegin
-- plan is how the user saves for the goal (models.GoalPlan as JSON): a
-- monthly amount put aside, saving the whole price or a down payment with
-- the rest in installments. NULL without a plan.
ALTER TABLE goals ADD COLUMN plan JSON;

-- loan_goal_id links the payments of the loan a goal was bought with (plain
-- expenses from a bank account, not in or out of the goal's account) to the
-- goal, so they show as its transactions and paying them pays it off.
ALTER TABLE transactions ADD COLUMN loan_goal_id VARCHAR REFERENCES goals (id) ON DELETE SET NULL;
CREATE INDEX transactions_loan_goal ON transactions (loan_goal_id) WHERE loan_goal_id IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX transactions_loan_goal;
ALTER TABLE transactions DROP COLUMN loan_goal_id;
ALTER TABLE goals DROP COLUMN plan;
-- +goose StatementEnd
