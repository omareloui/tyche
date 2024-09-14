package repository

import (
	"context"

	"github.com/omareloui/financial-plans/internal/models"
)

func (r *Repository) GetBalances(ctx context.Context) ([]models.Balance, error) {
	dbBalances, err := r.db.GetBalances(ctx)
	if err != nil {
		return nil, err
	}
	return models.DBBalancesToBalances(dbBalances), nil
}
