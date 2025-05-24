package repository

import (
	"context"
	"database/sql"

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

func (r *Repository) GetAllCurrencies(ctx context.Context) ([]models.Currency, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbCurrencies, err := q.GetAllCurrencies(ctx)
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
	return dbCurrencyToCurrency(&dbCurrency.Currency, dbCurrency.IsDefault.(int64) != 0), nil
}

func (r *Repository) GetCurrencyByString(ctx context.Context, cur string) (*models.Currency, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbCurrency, err := q.GetCurrencyByString(ctx, cur)
	if err != nil {
		return nil, err
	}
	return dbCurrencyToCurrency(&dbCurrency.Currency, dbCurrency.IsDefault.(int64) != 0), nil
}

func (r *Repository) GetDefaultCurrency(ctx context.Context) (*models.Currency, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbCurrency, err := q.GetDefaultCurrency(ctx)
	if err != nil {
		return nil, err
	}
	return dbCurrencyToCurrency(&dbCurrency.Currency, dbCurrency.IsDefault.(int64) != 0), nil
}

func (r *Repository) createCurrency(ctx context.Context, curr models.Currency) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	l := logger.FromCtx(ctx)

	if err := q.CreateCurrency(ctx, queries.CreateCurrencyParams{
		ID:         curr.ID,
		String:     curr.String,
		Symbol:     curr.Symbol,
		Kind:       string(curr.Kind),
		IbanPrefix: sql.NullString{String: curr.IbanPrefix, Valid: curr.IbanPrefix != ""},
		IsShown:    curr.IsShown,
	}); err != nil {
		l.Error("error creating currency", zap.Any("curr", curr), zap.Error(err))
		return err
	}

	l.Debug("created currency", zap.Any("curr", curr))
	return nil
}

func (r *Repository) SetCurrencyIsShown(ctx context.Context, currId string, isShown bool) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	l := logger.FromCtx(ctx)

	if err := q.SetCurrencyShown(ctx, queries.SetCurrencyShownParams{
		ID:      currId,
		IsShown: isShown,
	}); err != nil {
		l.Error("error setting IsShown for currency", zap.String("currId", currId), zap.Bool("is_shown", isShown), zap.Error(err))
		return err
	}

	l.Debug("set currency shown", zap.String("currId", currId), zap.Bool("is_shown", isShown))
	return nil
}

func (r *Repository) SetDefaultCurrency(ctx context.Context, currId string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	l := logger.FromCtx(ctx)

	if err := q.SetDefaultCurrency(ctx, currId); err != nil {
		l.Error("error setting default currency", zap.String("currId", currId), zap.Error(err))
		return err
	}

	l.Debug("set default currency", zap.String("currId", currId))
	return nil
}

func dbCurrencyToCurrency(dbCurr *queries.Currency, isDefault bool) *models.Currency {
	return &models.Currency{
		ID:         dbCurr.ID,
		String:     dbCurr.String,
		Symbol:     dbCurr.Symbol,
		Kind:       models.CurrencyKind(dbCurr.Kind),
		IbanPrefix: dbCurr.IbanPrefix.String,
		IsShown:    dbCurr.IsShown,
		IsDefault:  isDefault,
	}
}

func dbCurrenciesRowToCurrencies[T queries.GetAllCurrenciesRow | queries.GetCurrenciesRow](dbCurrency []T) []models.Currency {
	currencies := make([]models.Currency, len(dbCurrency))
	for i, cur := range dbCurrency {
		_cur := queries.GetCurrenciesRow(cur)
		currencies[i] = *dbCurrencyToCurrency(&_cur.Currency, _cur.IsDefault.(int64) != 0)
	}
	return currencies
}
