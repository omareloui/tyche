-- name: ListPushTenants :many
SELECT DISTINCT tenant_id FROM push_subscriptions;

-- name: ListPushSubscriptions :many
SELECT * FROM push_subscriptions WHERE tenant_id = sqlc.arg(tenant_id);

-- name: UpsertPushSubscription :exec
INSERT INTO push_subscriptions (endpoint, tenant_id, p256dh, auth, sources)
VALUES (sqlc.arg(endpoint), sqlc.arg(tenant_id), sqlc.arg(p256dh), sqlc.arg(auth), sqlc.arg(sources))
ON CONFLICT (endpoint) DO UPDATE SET
    tenant_id = excluded.tenant_id,
    p256dh = excluded.p256dh,
    auth = excluded.auth,
    sources = excluded.sources,
    -- Sent again only when what it counts changed.
    last_sent = CASE
        WHEN push_subscriptions.sources = excluded.sources AND push_subscriptions.tenant_id = excluded.tenant_id
            THEN push_subscriptions.last_sent
        ELSE -1
    END;

-- name: SetPushSubscriptionSent :exec
UPDATE push_subscriptions SET last_sent = sqlc.arg(last_sent) WHERE endpoint = sqlc.arg(endpoint);

-- name: DeletePushSubscription :exec
DELETE FROM push_subscriptions WHERE endpoint = sqlc.arg(endpoint) AND tenant_id = sqlc.arg(tenant_id);

-- name: DeleteGonePushSubscription :exec
DELETE FROM push_subscriptions WHERE endpoint = sqlc.arg(endpoint);

-- name: DeleteTenantPushSubscriptions :exec
DELETE FROM push_subscriptions WHERE tenant_id = sqlc.arg(tenant_id);
