-- +goose Up
-- +goose StatementBegin
-- Tags group transactions across categories and accounts ("Dubai trip",
-- "Wedding"). A tag with days is a trip: it can tag everything in them at once
-- and, with auto_tag, the new transactions dated in them.
CREATE TABLE tags (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR NOT NULL,
    color VARCHAR NOT NULL DEFAULT 'gray', -- one of models.CategoryColors
    starts_on DATE,
    ends_on DATE,
    auto_tag BOOLEAN NOT NULL DEFAULT 0,
    budget FLOAT,
    budget_currency_id VARCHAR REFERENCES currencies (id),
    archived_at TIMESTAMP,
    note VARCHAR NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX tags_name ON tags (name COLLATE NOCASE);

CREATE TABLE transaction_tags (
    transaction_id VARCHAR NOT NULL REFERENCES transactions (id) ON DELETE CASCADE,
    tag_id VARCHAR NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    PRIMARY KEY (transaction_id, tag_id)
);
CREATE INDEX transaction_tags_tag ON transaction_tags (tag_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE transaction_tags;
DROP TABLE tags;
-- +goose StatementEnd
