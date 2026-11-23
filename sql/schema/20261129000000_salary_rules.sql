-- +goose Up
-- +goose StatementBegin
-- The user's own tax rules for the salary calculator (payroll.Rules as JSON,
-- its currency inside), typed or copied from the built in / AI ones.
CREATE TABLE salary_rules (
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
DROP TABLE salary_rules;
-- +goose StatementEnd
