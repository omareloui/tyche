-- name: GetVisaRules :many
SELECT * FROM visa_rules ORDER BY name COLLATE NOCASE, id;

-- name: GetVisaRule :one
SELECT * FROM visa_rules WHERE id = sqlc.arg(id);

-- name: CreateVisaRule :exec
INSERT INTO visa_rules (id, name, country, rules)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(country), sqlc.arg(rules));

-- name: UpdateVisaRule :exec
UPDATE visa_rules
SET name = sqlc.arg(name), country = sqlc.arg(country), rules = sqlc.arg(rules), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: DeleteVisaRule :exec
DELETE FROM visa_rules WHERE id = sqlc.arg(id);
