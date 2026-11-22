-- +goose Up
-- +goose StatementBegin
-- The template the AI guessed for an unparsed sms (JSON), verified to read
-- it, prefilling the "Make template" form; NULL while none was asked for.
ALTER TABLE sms_messages ADD COLUMN template_guess VARCHAR;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE sms_messages DROP COLUMN template_guess;
-- +goose StatementEnd
