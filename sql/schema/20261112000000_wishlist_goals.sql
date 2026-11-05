-- +goose Up
-- +goose StatementBegin
-- goal_id is the goal saving for a wishlist item (models.WishlistSaving),
-- whose purchase, undo or deletion decides the item.
ALTER TABLE wishlist_items ADD COLUMN goal_id VARCHAR REFERENCES goals (id) ON DELETE SET NULL;
CREATE INDEX wishlist_items_goal ON wishlist_items (goal_id) WHERE goal_id IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX wishlist_items_goal;
ALTER TABLE wishlist_items DROP COLUMN goal_id;
-- +goose StatementEnd
