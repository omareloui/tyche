-- +goose Up
-- +goose StatementBegin
-- The visa readiness tool's requirements the AI looked up for a country's
-- visa of a year (visaguess.Key), shared by every user as they're public;
-- kept under what was typed and under the visa's official name. A user
-- refetching them replaces the row.
CREATE TABLE visa_rules_cache (
    key VARCHAR PRIMARY KEY,
    rules TEXT NOT NULL,
    model VARCHAR NOT NULL,
    created_at DATETIME NOT NULL
) WITHOUT ROWID;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE visa_rules_cache;
-- +goose StatementEnd
