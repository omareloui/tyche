-- name: ListSMSTemplates :many
SELECT sqlc.embed(sms_templates) FROM sms_templates
ORDER BY priority DESC, created_at, id;

-- name: GetSMSTemplate :one
SELECT sqlc.embed(sms_templates) FROM sms_templates
WHERE id = sqlc.arg(id);

-- name: CreateSMSTemplate :exec
INSERT INTO sms_templates (
    id,
    name,
    bank,
    senders,
    action,
    tx_kind,
    body,
    fixed,
    sample,
    priority,
    is_enabled,
    seed_key
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(name),
    sqlc.arg(bank),
    sqlc.arg(senders),
    sqlc.arg(action),
    sqlc.narg(tx_kind),
    sqlc.arg(body),
    sqlc.arg(fixed),
    sqlc.arg(sample),
    sqlc.arg(priority),
    sqlc.arg(is_enabled),
    sqlc.narg(seed_key)
)
ON CONFLICT (seed_key) DO NOTHING;

-- name: AdoptSMSTemplateSeed :execrows
UPDATE sms_templates
SET seed_key = sqlc.arg(seed_key)
WHERE id = (
    SELECT t.id FROM sms_templates AS t
    WHERE t.seed_key IS NULL AND t.body = sqlc.arg(body)
    ORDER BY t.created_at, t.id
    LIMIT 1
)
AND NOT EXISTS (SELECT 1 FROM sms_templates AS k WHERE k.seed_key = sqlc.arg(seed_key));

-- name: UpdateSMSTemplate :exec
UPDATE sms_templates
SET
    name = sqlc.arg(name),
    bank = sqlc.arg(bank),
    senders = sqlc.arg(senders),
    action = sqlc.arg(action),
    tx_kind = sqlc.narg(tx_kind),
    body = sqlc.arg(body),
    fixed = sqlc.arg(fixed),
    sample = sqlc.arg(sample),
    priority = sqlc.arg(priority),
    is_enabled = sqlc.arg(is_enabled),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: DeleteSMSTemplate :exec
DELETE FROM sms_templates WHERE id = sqlc.arg(id);

-- name: GetMeta :one
SELECT value FROM meta WHERE key = sqlc.arg(key);

-- name: SetMeta :exec
INSERT INTO meta (key, value) VALUES (sqlc.arg(key), sqlc.arg(value))
ON CONFLICT (key) DO UPDATE SET value = excluded.value;
