package repository

import (
	"context"

	"github.com/omareloui/financial-plans/internal/models"
)

func (r *Repository) GetCurrencies(ctx context.Context) ([]models.Currency, error) {
	dbCurrencies, err := r.db.GetCurrencies(ctx)
	if err != nil {
		return nil, err
	}
	return models.DBCurrenciesToCurrencies(dbCurrencies), nil
}
