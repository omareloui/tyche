-- +goose Up
-- +goose StatementBegin
ALTER TABLE accounts ADD COLUMN sms_ref VARCHAR;
ALTER TABLE transactions ADD COLUMN needs_review BOOLEAN NOT NULL DEFAULT FALSE;
CREATE TABLE devices (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR NOT NULL,
    last_seen_at TIMESTAMP,
    revoked_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE sms_messages (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    sender VARCHAR NOT NULL,
    body VARCHAR NOT NULL,
    received_at TIMESTAMP NOT NULL,
    hash VARCHAR UNIQUE NOT NULL,
    status VARCHAR NOT NULL,
    error VARCHAR,
    transaction_id VARCHAR,
    device_id VARCHAR,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE SET NULL,
    FOREIGN KEY (device_id) REFERENCES devices (id) ON DELETE SET NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE sms_messages;
DROP TABLE devices;
ALTER TABLE transactions DROP COLUMN needs_review;
ALTER TABLE accounts DROP COLUMN sms_ref;
-- +goose StatementEnd
