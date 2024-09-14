package repository

import (
	"context"

	"github.com/omareloui/financial-plans/internal/db"
	"github.com/omareloui/financial-plans/internal/models"
)

func (r *Repository) GetBalances(ctx context.Context) ([]models.Balance, error) {
	dbBalances, err := r.db.GetBalances(ctx)
	if err != nil {
		return nil, err
	}
	return models.DBBalancesToBalances(dbBalances), nil
}

func (r *Repository) AddToBalance(ctx context.Context, currencyID string, amount float64) error {
	err := r.db.AddToBalance(ctx, db.AddToBalanceParams{CurrencyID: currencyID, Amount: amount})
	if err != nil {
		return err
	}
	return nil
}

func (r *Repository) RemoveFromBalance(ctx context.Context, currencyID string, amount float64) error {
	err := r.db.RemoveFromBalance(ctx, db.RemoveFromBalanceParams{CurrencyID: currencyID, Amount: amount})
	if err != nil {
		return err
	}
	return nil
}
