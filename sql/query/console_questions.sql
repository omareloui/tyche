-- name: GetConsoleQuestionByText :one
SELECT * FROM console_questions WHERE lower(question) = lower(sqlc.arg(question));

-- name: GetConsoleQuestion :one
SELECT * FROM console_questions WHERE id = sqlc.arg(id);

-- name: ListConsoleQuestions :many
SELECT * FROM console_questions ORDER BY created_at DESC, id DESC LIMIT sqlc.arg(lim);

-- name: SaveConsoleQuestion :exec
-- Asked again, a question takes the new SQL and moves to the top.
INSERT INTO console_questions (id, question, sql, explain, chart)
VALUES (sqlc.arg(id), sqlc.arg(question), sqlc.arg(sql), sqlc.arg(explain), sqlc.arg(chart))
ON CONFLICT (lower(question)) DO UPDATE SET
    sql = excluded.sql, explain = excluded.explain, chart = excluded.chart, created_at = CURRENT_TIMESTAMP;

-- name: TouchConsoleQuestion :exec
UPDATE console_questions SET created_at = CURRENT_TIMESTAMP WHERE id = sqlc.arg(id);

-- name: DeleteConsoleQuestion :exec
DELETE FROM console_questions WHERE id = sqlc.arg(id);

-- name: PruneConsoleQuestions :exec
-- Keeps the latest ones.
DELETE FROM console_questions WHERE id NOT IN (
    SELECT id FROM console_questions ORDER BY created_at DESC, id DESC LIMIT sqlc.arg(keep)
);
