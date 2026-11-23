-- name: GetVisaRules :one
SELECT rules, model, created_at FROM visa_rules_cache WHERE key = sqlc.arg(key);

-- name: SetVisaRules :exec
INSERT INTO visa_rules_cache (key, rules, model, created_at)
VALUES (sqlc.arg(key), sqlc.arg(rules), sqlc.arg(model), sqlc.arg(created_at))
ON CONFLICT (key) DO UPDATE SET rules = excluded.rules, model = excluded.model, created_at = excluded.created_at;
