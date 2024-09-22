package repository

import (
	"context"

	"github.com/omareloui/financial-planner/internal/db"
	"github.com/omareloui/financial-planner/internal/models"
)

func (r *Repository) GetCurrencies(ctx context.Context) ([]models.Currency, error) {
	dbCurrencies, err := r.q.GetCurrencies(ctx)
	if err != nil {
		return nil, err
	}
	return dbCurrenciesToCurrencies(dbCurrencies), nil
}

func (r *Repository) GetCurrency(ctx context.Context, id string) (*models.Currency, error) {
	dbCurrency, err := r.q.GetCurrency(ctx, id)
	if err != nil {
		return nil, err
	}
	return dbCurrencyToCurrency(&dbCurrency), nil
}

func dbCurrencyToCurrency(dbCurr *db.Currency) *models.Currency {
	return &models.Currency{
		ID:        dbCurr.ID,
		Enum:      models.CurrencyEnum(dbCurr.Enum),
		EgpFxRate: dbCurr.EgpFxRate,
	}
}

func dbCurrenciesToCurrencies(dbCurrency []db.Currency) []models.Currency {
	currencies := make([]models.Currency, len(dbCurrency))
	for i, cur := range dbCurrency {
		currencies[i] = *dbCurrencyToCurrency(&cur)
	}
	return currencies
}
