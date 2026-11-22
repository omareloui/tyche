-- +goose Up
-- +goose StatementBegin
-- What a Gemini call read: a receipt or an sms (guessing its template), each
-- kind counted toward its own monthly limit.
ALTER TABLE receipt_scans ADD COLUMN kind VARCHAR NOT NULL DEFAULT 'receipt';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE receipt_scans DROP COLUMN kind;
-- +goose StatementEnd
