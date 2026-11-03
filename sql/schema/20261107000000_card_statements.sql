-- +goose Up
-- +goose StatementBegin
-- A credit card's statement cycle: it closes on statement_day (1-31, clamped
-- to the month's length) and its balance is due due_days after, or on
-- due_day of the following month (one of them is set with statement_day).
ALTER TABLE accounts ADD COLUMN statement_day INTEGER;
ALTER TABLE accounts ADD COLUMN due_days INTEGER;
ALTER TABLE accounts ADD COLUMN due_day INTEGER;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE accounts DROP COLUMN due_day;
ALTER TABLE accounts DROP COLUMN due_days;
ALTER TABLE accounts DROP COLUMN statement_day;
-- +goose StatementEnd
