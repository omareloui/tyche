-- +goose Up
-- +goose StatementBegin
-- rating is the merchant's 1 to 5 stars, NULL when unrated.
ALTER TABLE counterparties ADD COLUMN rating INTEGER CHECK (rating BETWEEN 1 AND 5);
-- note is free text about any counterparty, merchant or person.
ALTER TABLE counterparties ADD COLUMN note VARCHAR NOT NULL DEFAULT '';
-- The branches of a merchant, in the order they're listed.
CREATE TABLE counterparty_locations (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    counterparty_id VARCHAR NOT NULL REFERENCES counterparties (id) ON DELETE CASCADE,
    label VARCHAR NOT NULL,
    address VARCHAR NOT NULL DEFAULT '',
    maps_url VARCHAR NOT NULL DEFAULT '',
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX counterparty_locations_cp ON counterparty_locations (counterparty_id, position);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE counterparty_locations;
ALTER TABLE counterparties DROP COLUMN note;
ALTER TABLE counterparties DROP COLUMN rating;
-- +goose StatementEnd
