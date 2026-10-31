-- +goose Up
-- +goose StatementBegin
-- The user's own templates reading bank sms, tried before the shared ones.
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
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- The template that read it, no foreign key as it can be a shared one.
ALTER TABLE sms_messages ADD COLUMN template_id VARCHAR;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE sms_messages DROP COLUMN template_id;
DROP TABLE sms_templates;
-- +goose StatementEnd
