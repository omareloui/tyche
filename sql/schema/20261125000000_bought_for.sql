-- +goose Up
-- +goose StatementBegin
-- A debt borrowed from someone who bought the user something points at the
-- expense of what they bought (several people can chip in): the debt is listed
-- under it. Deleting the expense leaves the debt on its own.
ALTER TABLE transactions ADD COLUMN bought_for_id VARCHAR REFERENCES transactions (id) ON DELETE SET NULL;
CREATE INDEX transactions_bought_for_id ON transactions (bought_for_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX transactions_bought_for_id;
ALTER TABLE transactions DROP COLUMN bought_for_id;
-- +goose StatementEnd
