package repository

import (
	"context"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

func (r *Repository) GetCurrencies(ctx context.Context) ([]models.Currency, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbCurrencies, err := q.GetCurrencies(ctx)
	if err != nil {
		return nil, err
	}
	return dbCurrenciesRowToCurrencies(dbCurrencies), nil
}

func (r *Repository) GetCurrency(ctx context.Context, id string) (*models.Currency, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbCurrency, err := q.GetCurrency(ctx, id)
	if err != nil {
		return nil, err
	}
	return dbCurrencyToCurrency(&dbCurrency.Currency), nil
}

func (r *Repository) GetCurrencyByEnum(ctx context.Context, enum models.CurrencyEnum) (*models.Currency, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbCurrency, err := q.GetCurrencyByEnum(ctx, int64(enum))
	if err != nil {
		return nil, err
	}
	return dbCurrencyToCurrency(&dbCurrency.Currency), nil
}

func (r *Repository) createCurrency(ctx context.Context, curr models.Currency) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	l := logger.FromCtx(ctx)

	if err := q.CreateCurrency(ctx, queries.CreateCurrencyParams{
		ID:   curr.ID,
		Enum: int64(curr.Enum),
	}); err != nil {
		l.Error("error creating currency", zap.Any("curr", curr), zap.Error(err))
		return err
	}

	l.Debug("created currency", zap.Any("curr", curr))
	return nil
}

func dbCurrencyToCurrency(dbCurr *queries.Currency) *models.Currency {
	return &models.Currency{
		ID:   dbCurr.ID,
		Enum: models.CurrencyEnum(dbCurr.Enum),
	}
}

func dbCurrenciesRowToCurrencies(dbCurrency []queries.GetCurrenciesRow) []models.Currency {
	currencies := make([]models.Currency, len(dbCurrency))
	for i, cur := range dbCurrency {
		currencies[i] = *dbCurrencyToCurrency(&cur.Currency)
	}
	return currencies
}
