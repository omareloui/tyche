-- +goose Up
-- +goose StatementBegin
-- How a charge of a split bill is shared (models.ChargeSplitMode): 0 by
-- default (by share when it has a rate, else equally), 1 by share, 2 equally.
ALTER TABLE transaction_items ADD COLUMN split_mode INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE transaction_items DROP COLUMN split_mode;
-- +goose StatementEnd
