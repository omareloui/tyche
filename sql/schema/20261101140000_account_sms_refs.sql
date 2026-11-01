-- +goose Up
-- +goose StatementBegin
-- The ways bank sms refer to the account (account number, card, ...).
ALTER TABLE accounts ADD COLUMN sms_refs JSON NOT NULL DEFAULT '[]';
UPDATE accounts SET sms_refs = json_array(sms_ref) WHERE sms_ref IS NOT NULL AND sms_ref != '';
ALTER TABLE accounts DROP COLUMN sms_ref;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE accounts ADD COLUMN sms_ref VARCHAR;
UPDATE accounts SET sms_ref = json_extract(sms_refs, '$[0]');
ALTER TABLE accounts DROP COLUMN sms_refs;
-- +goose StatementEnd
