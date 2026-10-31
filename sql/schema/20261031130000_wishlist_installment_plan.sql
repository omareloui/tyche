-- +goose Up
-- +goose StatementBegin
-- How a wishlist item is paid in installments (models.InstallmentPlan:
-- strategy, rate, down payment, fees, ...), NULL when paid cash or for items
-- from before plans, which only have installments and installment_amount.
ALTER TABLE wishlist_items ADD COLUMN installment_plan JSON;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE wishlist_items DROP COLUMN installment_plan;
-- +goose StatementEnd
