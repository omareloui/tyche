-- +goose Up
-- +goose StatementBegin
-- The salary calculator's tax rules the AI looked up for a country (and
-- region) of a tax year (taxguess.Key), shared by every user as they're
-- public; a user refetching them replaces the row.
CREATE TABLE tax_rules_cache (
    key VARCHAR PRIMARY KEY,
    rules TEXT NOT NULL,
    model VARCHAR NOT NULL,
    created_at DATETIME NOT NULL
) WITHOUT ROWID;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE tax_rules_cache;
-- +goose StatementEnd
