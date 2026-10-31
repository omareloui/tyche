-- +goose Up
-- +goose StatementBegin
-- The time zone of the dates and times in the user's bank sms.
ALTER TABLE user ADD COLUMN timezone VARCHAR NOT NULL DEFAULT 'Africa/Cairo';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE user DROP COLUMN timezone;
-- +goose StatementEnd
