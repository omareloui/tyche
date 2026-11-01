-- +goose Up
-- +goose StatementBegin
-- The profile picture, downloaded on signing in and served from /avatar (the
-- provider's url is rate limited). picture_hash versions its url.
ALTER TABLE user ADD COLUMN picture_data BLOB;
ALTER TABLE user ADD COLUMN picture_type TEXT;
ALTER TABLE user ADD COLUMN picture_hash TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE user DROP COLUMN picture_hash;
ALTER TABLE user DROP COLUMN picture_type;
ALTER TABLE user DROP COLUMN picture_data;
-- +goose StatementEnd
