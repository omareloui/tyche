-- +goose Up
CREATE INDEX transactions_scheduled_to ON transactions (scheduled_to, created_at);

-- +goose Down
DROP INDEX transactions_scheduled_to;
