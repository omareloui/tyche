-- +goose Up
-- +goose StatementBegin
-- When the user was asked whether to encrypt their database (with a
-- password), asked once on signing in while NULL.
ALTER TABLE user ADD COLUMN encryption_prompted_at TIMESTAMP;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE user DROP COLUMN encryption_prompted_at;
-- +goose StatementEnd
