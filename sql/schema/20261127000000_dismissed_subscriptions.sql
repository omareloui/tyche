-- +goose Up
-- +goose StatementBegin
-- The subscriptions the user dismissed from the "Looks recurring"
-- suggestions (models.DetectSubscriptions): one per merchant and currency,
-- with the typical amount it had then, so it's suggested again only when the
-- amount changes.
CREATE TABLE dismissed_subscriptions (
    counterparty_id VARCHAR NOT NULL REFERENCES counterparties (id) ON DELETE CASCADE,
    currency_id VARCHAR NOT NULL REFERENCES currencies (id) ON DELETE CASCADE,
    amount FLOAT NOT NULL,
    dismissed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (counterparty_id, currency_id)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE dismissed_subscriptions;
-- +goose StatementEnd
