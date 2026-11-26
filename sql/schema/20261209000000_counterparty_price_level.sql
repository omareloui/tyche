-- +goose Up
-- +goose StatementBegin
-- price_level is how pricey a merchant is, 1 ($) to 4 ($$$$), NULL when unset.
ALTER TABLE counterparties ADD COLUMN price_level INTEGER CHECK (price_level BETWEEN 1 AND 4);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE counterparties DROP COLUMN price_level;
-- +goose StatementEnd
