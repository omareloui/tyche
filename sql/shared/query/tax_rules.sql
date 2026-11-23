-- name: GetTaxRules :one
SELECT rules, model, created_at FROM tax_rules_cache WHERE key = sqlc.arg(key);

-- name: SetTaxRules :exec
INSERT INTO tax_rules_cache (key, rules, model, created_at)
VALUES (sqlc.arg(key), sqlc.arg(rules), sqlc.arg(model), sqlc.arg(created_at))
ON CONFLICT (key) DO UPDATE SET rules = excluded.rules, model = excluded.model, created_at = excluded.created_at;
