package repository

import (
	"context"

	"github.com/omareloui/tyche/internal/db"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"go.uber.org/zap"
)

func (r *Repository) GetItems(ctx context.Context) ([]models.Item, error) {
	l := logger.FromCtx(ctx)
	dbIts, err := r.q.GetItems(ctx)
	if err != nil {
		l.Error("fetching goals items", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched goals items")
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
		PurchasedAt: dbIt.PurchasedAt.Time,
		Currency: models.Currency{
			ID:   dbIt.CurrencyID,
			Enum: models.CurrencyEnum(dbIt.Enum.Int64),
		},
		CreatedAt: dbIt.CreatedAt,
	}
}

func dbItemsRowToItems(dbIts []db.GetItemsRow) []models.Item {
	items := make([]models.Item, len(dbIts))
	for i, it := range dbIts {
		items[i] = models.Item{
			ID:          it.ID,
			Name:        it.Name,
			Price:       it.Price,
			PurchasedAt: it.PurchasedAt.Time,
			Currency: models.Currency{
				ID:   it.CurrencyID,
				Enum: models.CurrencyEnum(it.Enum.Int64),
			},
			CreatedAt: it.CreatedAt,
		}
	}
	return items
}
