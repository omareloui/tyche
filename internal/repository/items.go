package repository

import (
	"context"

	"github.com/omareloui/financial-planner/internal/db"
	"github.com/omareloui/financial-planner/internal/models"
)

func (r *Repository) GetItems(ctx context.Context) ([]models.Item, error) {
	dbIts, err := r.q.GetItems(ctx)
	if err != nil {
		return nil, err
	}
	return models.DBItemsRowToItems(dbIts), nil
}

func (r *Repository) GetItem(ctx context.Context, id string) (*models.Item, error) {
	dbIt, err := r.q.GetItem(ctx, id)
	if err != nil {
		return nil, err
	}
	return models.DBItemRowToItem(&dbIt), nil
}

func (r *Repository) UpdateItem(ctx context.Context, it *models.Item) error {
	return r.q.UpdateItem(ctx, db.UpdateItemParams{
		ID:         it.ID,
		Name:       it.Name,
		Amount:     it.Amount,
		CurrencyID: it.Currency.ID,
	})
}

func (r *Repository) CreateItem(ctx context.Context, it *models.Item) error {
	return r.q.CreateItem(ctx, db.CreateItemParams{
		ID:         it.ID,
		Name:       it.Name,
		CurrencyID: it.Currency.ID,
		Amount:     it.Amount,
	})
}

func (r *Repository) DeleteItem(ctx context.Context, id string) error {
	return r.q.DeleteItem(ctx, id)
}
