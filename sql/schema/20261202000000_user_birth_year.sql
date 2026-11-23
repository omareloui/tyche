-- +goose Up
-- +goose StatementBegin
-- The user's birth year (set on /settings' profile card), for the FIRE
-- calculator's ages. NULL = not set.
ALTER TABLE user ADD COLUMN birth_year INTEGER;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE user DROP COLUMN birth_year;
-- +goose StatementEnd
