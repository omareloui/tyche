-- +goose Up
-- +goose StatementBegin
-- Templates reading bank sms for every user, seeded from smsparser.Seeds.
CREATE TABLE sms_templates (
    id VARCHAR NOT NULL PRIMARY KEY,
    name VARCHAR NOT NULL,
    bank VARCHAR NOT NULL DEFAULT '',
    senders JSON NOT NULL DEFAULT '[]',
    action VARCHAR NOT NULL,
    tx_kind INTEGER,
    body VARCHAR NOT NULL,
    fixed JSON NOT NULL DEFAULT '{}',
    sample VARCHAR NOT NULL DEFAULT '',
    priority INTEGER NOT NULL DEFAULT 0,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    seed_key VARCHAR UNIQUE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE meta (
    key VARCHAR NOT NULL PRIMARY KEY,
    value VARCHAR NOT NULL
) WITHOUT ROWID;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE meta;
DROP TABLE sms_templates;
-- +goose StatementEnd
