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
	return dbItemsRowToItems(dbIts), nil
}

func (r *Repository) GetItem(ctx context.Context, id string) (*models.Item, error) {
	dbIt, err := r.q.GetItem(ctx, id)
	if err != nil {
		return nil, err
	}
	return dbItemRowToItem(&dbIt), nil
}

func (r *Repository) UpdateItem(ctx context.Context, it *models.Item) error {
	return r.q.UpdateItem(ctx, db.UpdateItemParams{
		ID:         it.ID,
		Name:       it.Name,
		Price:      it.Price,
		CurrencyID: it.Currency.ID,
	})
}

func (r *Repository) CreateItem(ctx context.Context, it *models.Item) error {
	return r.q.CreateItem(ctx, db.CreateItemParams{
		ID:         it.ID,
		Name:       it.Name,
		CurrencyID: it.Currency.ID,
		Price:      it.Price,
	})
}

func (r *Repository) DeleteItem(ctx context.Context, id string) error {
	return r.q.DeleteItem(ctx, id)
}

func dbItemRowToItem(dbIt *db.GetItemRow) *models.Item {
	return &models.Item{
		ID:          dbIt.ID,
		Name:        dbIt.Name,
		Price:       dbIt.Price,
		PurchasedOn: dbIt.PurchasedOn.Time,
		Currency: models.Currency{
			ID:        dbIt.CurrencyID,
			Enum:      models.CurrencyEnum(dbIt.Enum.Int64),
			EgpFxRate: dbIt.EgpFxRate.Float64,
		},
	}
}

func dbItemsRowToItems(dbIts []db.GetItemsRow) []models.Item {
	items := make([]models.Item, len(dbIts))
	for i, it := range dbIts {
		items[i] = models.Item{
			ID:          it.ID,
			Name:        it.Name,
			Price:       it.Price,
			PurchasedOn: it.PurchasedOn.Time,
			Currency: models.Currency{
				ID:        it.CurrencyID,
				Enum:      models.CurrencyEnum(it.Enum.Int64),
				EgpFxRate: it.EgpFxRate.Float64,
			},
		}
	}
	return items
}
