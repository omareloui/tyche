-- name: ListDeletedTenants :many
SELECT tenant_id FROM deleted_tenants;

-- name: MarkTenantDeleted :exec
INSERT INTO deleted_tenants (tenant_id, deleted_at) VALUES (sqlc.arg(tenant_id), sqlc.arg(deleted_at))
ON CONFLICT (tenant_id) DO UPDATE SET deleted_at = excluded.deleted_at;

-- name: UnmarkTenantDeleted :exec
DELETE FROM deleted_tenants WHERE tenant_id = sqlc.arg(tenant_id);
