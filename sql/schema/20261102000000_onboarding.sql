-- +goose Up
-- +goose StatementBegin
-- When the user finished (or skipped) the onboarding at /welcome, shown on
-- signing in while NULL. It replaces the encryption question, now one of its
-- steps: users already there are done.
ALTER TABLE user ADD COLUMN onboarded_at TIMESTAMP;
UPDATE user SET onboarded_at = COALESCE(encryption_prompted_at, created_at);
ALTER TABLE user DROP COLUMN encryption_prompted_at;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE user ADD COLUMN encryption_prompted_at TIMESTAMP;
UPDATE user SET encryption_prompted_at = onboarded_at;
ALTER TABLE user DROP COLUMN onboarded_at;
-- +goose StatementEnd
