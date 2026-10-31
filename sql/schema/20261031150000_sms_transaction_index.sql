-- +goose Up
-- Transaction lists look up each row's sms (body, from-sms filter).
CREATE INDEX sms_messages_transaction_id ON sms_messages (transaction_id);

-- +goose Down
DROP INDEX sms_messages_transaction_id;
