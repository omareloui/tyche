-- name: CreateGoalProgress :exec
INSERT INTO goals_progresses (
    goal_id,
    transaction_id
)
VALUES (?, ?);
