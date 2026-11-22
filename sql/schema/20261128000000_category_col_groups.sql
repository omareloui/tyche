-- +goose Up
-- +goose StatementBegin
-- The price group (models.ColGroup: food, housing, transport, ...) an expense
-- category's spending is weighed by in the cost of living calculator, when
-- the user picked one; categories without a row are guessed from their name.
CREATE TABLE category_col_groups (
    category_id VARCHAR PRIMARY KEY REFERENCES categories (id) ON DELETE CASCADE,
    col_group VARCHAR NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE category_col_groups;
-- +goose StatementEnd
