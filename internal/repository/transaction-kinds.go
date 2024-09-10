package repository

import (
	"context"

	"github.com/omareloui/financial-plans/internal/models"
)

func (r *Repository) GetTransactionKinds(ctx context.Context) ([]models.TransactionKind, error) {
	dbCurrencies, err := r.db.GetTransactionKinds(ctx)
	if err != nil {
		return nil, err
	}
	return models.DBTransactionKindsToTransactionKinds(dbCurrencies), nil
}
