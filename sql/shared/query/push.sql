-- name: ListPushTenants :many
SELECT DISTINCT tenant_id FROM push_subscriptions;

-- name: ListPushSubscriptions :many
SELECT * FROM push_subscriptions WHERE tenant_id = sqlc.arg(tenant_id);

-- name: UpsertPushSubscription :exec
INSERT INTO push_subscriptions (endpoint, tenant_id, p256dh, auth, sources, repost_hours)
VALUES (sqlc.arg(endpoint), sqlc.arg(tenant_id), sqlc.arg(p256dh), sqlc.arg(auth), sqlc.arg(sources), sqlc.arg(repost_hours))
ON CONFLICT (endpoint) DO UPDATE SET
    tenant_id = excluded.tenant_id,
    p256dh = excluded.p256dh,
    auth = excluded.auth,
    sources = excluded.sources,
    repost_hours = excluded.repost_hours,
    repost_at = CASE WHEN excluded.repost_hours = 0 THEN NULL ELSE push_subscriptions.repost_at END,
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

-- name: SetPushRepost :exec
-- A notification was dismissed: shown again repost_hours later, counted from
-- the first dismissal not reposted yet.
UPDATE push_subscriptions
SET repost_at = COALESCE(repost_at, sqlc.arg(now) + repost_hours * 3600)
WHERE endpoint = sqlc.arg(endpoint) AND repost_hours > 0 AND last_sent > 0;

-- name: ListDuePushReposts :many
SELECT * FROM push_subscriptions WHERE repost_at <= CAST(sqlc.arg(now) AS INTEGER);

-- name: ClearPushRepost :exec
UPDATE push_subscriptions SET repost_at = NULL WHERE endpoint = sqlc.arg(endpoint);
