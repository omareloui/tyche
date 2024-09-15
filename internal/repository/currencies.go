package repository

import (
	"context"

	"github.com/omareloui/financial-planner/internal/models"
)

func (r *Repository) GetCurrencies(ctx context.Context) ([]models.Currency, error) {
	dbCurrencies, err := r.q.GetCurrencies(ctx)
	if err != nil {
		return nil, err
	}
	return models.DBCurrenciesToCurrencies(dbCurrencies), nil
}

func (r *Repository) GetCurrency(ctx context.Context, id string) (*models.Currency, error) {
	dbCurrency, err := r.q.GetCurrency(ctx, id)
	if err != nil {
		return nil, err
	}
	return models.DBCurrencyToCurrency(&dbCurrency), nil
}

func (r *Repository) GetCurrenciesMap(ctx context.Context) (map[string]string, error) {
	currencies, err := r.GetCurrencies(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(currencies))
	for _, c := range currencies {
		m[c.ID] = c.Enum.String()
	}
	return m, nil
}

func (r *Repository) CurrenciesToMap(currencies []models.Currency) map[string]string {
	m := make(map[string]string, len(currencies))
	for _, c := range currencies {
		m[c.ID] = c.Enum.String()
	}
	return m
}
