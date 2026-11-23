-- +goose Up
-- +goose StatementBegin
-- The user's own visa requirements for the visa readiness tool (visa.Rule
-- as JSON, its currency inside), typed or copied from the built in / AI
-- ones.
CREATE TABLE visa_rules (
    id VARCHAR PRIMARY KEY,
    name VARCHAR NOT NULL,
    country VARCHAR NOT NULL,
    rules TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE visa_rules;
-- +goose StatementEnd
