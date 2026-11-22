-- name: GetCategoryColGroups :many
SELECT category_id, col_group FROM category_col_groups;

-- name: SetCategoryColGroup :exec
INSERT INTO category_col_groups (category_id, col_group)
VALUES (sqlc.arg(category_id), sqlc.arg(col_group))
ON CONFLICT (category_id) DO UPDATE SET col_group = excluded.col_group;

-- name: DeleteCategoryColGroup :exec
DELETE FROM category_col_groups WHERE category_id = sqlc.arg(category_id);
