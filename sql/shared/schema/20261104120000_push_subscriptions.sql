-- +goose Up
-- +goose StatementBegin
-- The Web Push subscriptions of the installed apps, one per browser, each
-- with the counts it wants on its badge (sources: comma separated sms,
-- review, overdue) and the total it was last sent.
CREATE TABLE push_subscriptions (
    endpoint VARCHAR NOT NULL PRIMARY KEY,
    tenant_id VARCHAR NOT NULL,
    p256dh VARCHAR NOT NULL,
    auth VARCHAR NOT NULL,
    sources VARCHAR NOT NULL,
    last_sent INTEGER NOT NULL DEFAULT -1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) WITHOUT ROWID;
CREATE INDEX push_subscriptions_tenant ON push_subscriptions (tenant_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE push_subscriptions;
-- +goose StatementEnd
