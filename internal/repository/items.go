package repository

import (
	"context"

	"github.com/omareloui/financial-planner/internal/db"
	"github.com/omareloui/financial-planner/internal/models"
)

func (r *Repository) GetItems(ctx context.Context) ([]models.Item, error) {
	dbIts, err := r.db.GetItems(ctx)
	if err != nil {
		return nil, err
	}
	return models.DBItemsRowToItems(dbIts), nil
}

func (r *Repository) GetItem(ctx context.Context, id string) (*models.Item, error) {
	dbIt, err := r.db.GetItem(ctx, id)
	if err != nil {
		return nil, err
	}
	return models.DBItemRowToItem(&dbIt), nil
}

func (r *Repository) UpdateItem(ctx context.Context, it *models.Item) error {
	return r.db.UpdateItem(ctx, db.UpdateItemParams{
		ID:         it.ID,
		Name:       it.Name,
		Amount:     it.Amount,
		CurrencyID: it.Currency.ID,
	})
}

func (r *Repository) CreateItem(ctx context.Context, it *models.Item) error {
	return r.db.CreateItem(ctx, db.CreateItemParams{
		ID:         it.ID,
		Name:       it.Name,
		CurrencyID: it.Currency.ID,
		Amount:     it.Amount,
	})
}

func (r *Repository) DeleteItem(ctx context.Context, id string) error {
	return r.db.DeleteItem(ctx, id)
}
