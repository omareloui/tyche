package repository

import (
	"context"

	"github.com/omareloui/tyche/internal/db"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"go.uber.org/zap"
)

func (r *Repository) GetTransactionKinds(ctx context.Context) ([]models.TransactionKind, error) {
	l := logger.FromCtx(ctx)
	dbKinds, err := r.q.GetTransactionKinds(ctx)
	if err != nil {
		l.Error("fetching tx kinds", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched transaction kinds")
	return dbTransactionKindsToTransactionKinds(dbKinds), nil
}

func (r *Repository) GetTransactionKind(ctx context.Context, id string) (*models.TransactionKind, error) {
	k, err := r.q.GetTransactionKind(ctx, id)
	if err != nil {
		return nil, err
	}
	return dbTransactionKindToTransactionKind(&k), nil
}

func dbTransactionKindToTransactionKind(dbKind *db.TransactionKind) *models.TransactionKind {
	return &models.TransactionKind{
		ID:              dbKind.ID,
		Enum:            models.TxKind(dbKind.Enum),
		IncreaseBalance: dbKind.IncreaseBalance.Bool,
	}
}

func dbTransactionKindsToTransactionKinds(dbKinds []db.TransactionKind) []models.TransactionKind {
	kinds := make([]models.TransactionKind, len(dbKinds))
	for i, kind := range dbKinds {
		kinds[i] = *dbTransactionKindToTransactionKind(&kind)
	}
	return kinds
}
