-- name: CreateSMSMessage :exec
INSERT INTO sms_messages (
    id,
    sender,
    body,
    received_at,
    hash,
    status,
    error,
    transaction_id,
    device_id,
    template_id
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(sender),
    sqlc.arg(body),
    sqlc.arg(received_at),
    sqlc.arg(hash),
    sqlc.arg(status),
    sqlc.arg(error),
    sqlc.arg(transaction_id),
    sqlc.arg(device_id),
    sqlc.arg(template_id)
);

-- name: GetSMSMessageByHash :one
SELECT
    sqlc.embed(sms_messages)
FROM sms_messages
WHERE hash = sqlc.arg(hash)
LIMIT 1;

-- name: GetSMSMessage :one
SELECT
    sqlc.embed(sms_messages)
FROM sms_messages
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: GetSMSMessagesByStatus :many
SELECT
    sqlc.embed(sms_messages)
FROM sms_messages
WHERE status IN (sqlc.slice(statuses))
ORDER BY received_at DESC;

-- name: UpdateSMSMessageResult :exec
UPDATE sms_messages
SET
    status = sqlc.arg(status),
    error = sqlc.arg(error),
    transaction_id = sqlc.arg(transaction_id),
    template_id = sqlc.arg(template_id)
WHERE id = sqlc.arg(id);

-- name: CountSMSMessagesByStatus :one
SELECT COUNT(*) FROM sms_messages
WHERE status IN (sqlc.slice(statuses));

-- name: IgnoreSMSMessagesByStatus :execrows
UPDATE sms_messages
SET status = 'ignored'
WHERE status IN (sqlc.slice(statuses));
