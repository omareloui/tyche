-- +goose Up
-- +goose StatementBegin
-- console_questions are the questions asked in the SQL console's Ask tab with
-- the SQL the AI wrote for them (that ran), so asking one again reruns it
-- without a call: explain is the AI's line about it, chart how its rows show
-- (number / table / bar).
CREATE TABLE console_questions (
    id TEXT PRIMARY KEY,
    question TEXT NOT NULL,
    sql TEXT NOT NULL,
    explain TEXT NOT NULL DEFAULT '',
    chart TEXT NOT NULL DEFAULT 'table',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX console_questions_question ON console_questions (lower(question));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE console_questions;
-- +goose StatementEnd
