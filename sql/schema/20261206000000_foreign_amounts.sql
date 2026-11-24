-- +goose Up
-- +goose StatementBegin
-- What a transaction was paid in when not its account's currency (a card
-- used abroad): original_amount of original_currency (its code), amount
-- being what the account moved. fx_fee is the part of amount the account's
-- foreign currency fee took (NULL = none).
ALTER TABLE transactions ADD COLUMN original_amount FLOAT;
ALTER TABLE transactions ADD COLUMN original_currency VARCHAR;
ALTER TABLE transactions ADD COLUMN fx_fee FLOAT;
-- The fee an account charges on a purchase in another currency: a percent
-- of the converted amount, at least fx_fee_min (in its currency).
ALTER TABLE accounts ADD COLUMN fx_fee_percent FLOAT;
ALTER TABLE accounts ADD COLUMN fx_fee_min FLOAT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE accounts DROP COLUMN fx_fee_min;
ALTER TABLE accounts DROP COLUMN fx_fee_percent;
ALTER TABLE transactions DROP COLUMN fx_fee;
ALTER TABLE transactions DROP COLUMN original_currency;
ALTER TABLE transactions DROP COLUMN original_amount;
-- +goose StatementEnd
