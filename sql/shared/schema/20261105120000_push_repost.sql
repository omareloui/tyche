-- +goose Up
-- +goose StatementBegin
-- Dismissed badge notifications still unhandled are shown again (silently)
-- repost_hours after the first dismissal (0 = never): repost_at is when,
-- unix seconds, NULL when nothing was dismissed.
ALTER TABLE push_subscriptions ADD COLUMN repost_hours INTEGER NOT NULL DEFAULT 1;
ALTER TABLE push_subscriptions ADD COLUMN repost_at INTEGER;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE push_subscriptions DROP COLUMN repost_at;
ALTER TABLE push_subscriptions DROP COLUMN repost_hours;
-- +goose StatementEnd
