-- name: GetSalaryRules :many
SELECT * FROM salary_rules ORDER BY name COLLATE NOCASE, id;

-- name: GetSalaryRule :one
SELECT * FROM salary_rules WHERE id = sqlc.arg(id);

-- name: CreateSalaryRule :exec
INSERT INTO salary_rules (id, name, country, rules)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(country), sqlc.arg(rules));

-- name: UpdateSalaryRule :exec
UPDATE salary_rules
SET name = sqlc.arg(name), country = sqlc.arg(country), rules = sqlc.arg(rules), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: DeleteSalaryRule :exec
DELETE FROM salary_rules WHERE id = sqlc.arg(id);
