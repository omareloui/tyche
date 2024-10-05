-- name: CreateGoalProgress :exec
INSERT INTO goals_progresses (
    goal_id,
    transaction_id
)
VALUES (
    sqlc.arg(goal_id),
    sqlc.arg(transaction_id)
);
