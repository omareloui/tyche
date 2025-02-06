package repository

import (
	"context"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

func (r *Repository) GetTransactionKinds(ctx context.Context) ([]models.TransactionKind, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		l.Error("getting db from context", zap.Error(err))
		return nil, err
	}

	dbKinds, err := q.GetTransactionKinds(ctx)
	if err != nil {
		l.Error("fetching tx kinds", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched transaction kinds")
	return dbTransactionKindsRowToTransactionKinds(dbKinds), nil
}

func (r *Repository) GetTransactionKind(ctx context.Context, id string) (*models.TransactionKind, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		l.Error("getting db from context", zap.Error(err))
		return nil, err
	}

	k, err := q.GetTransactionKind(ctx, id)
	if err != nil {
		return nil, err
	}
	return dbTransactionKindToTransactionKind(&k.TransactionKind), nil
}

func dbTransactionKindToTransactionKind(dbKind *queries.TransactionKind) *models.TransactionKind {
	return &models.TransactionKind{
		ID:              dbKind.ID,
		Enum:            models.TxKind(dbKind.Enum),
		IncreaseBalance: dbKind.IncreaseBalance.Bool,
	}
}

func dbTransactionKindsRowToTransactionKinds(dbKinds []queries.GetTransactionKindsRow) []models.TransactionKind {
	kinds := make([]models.TransactionKind, len(dbKinds))
	for i, kind := range dbKinds {
		kinds[i] = *dbTransactionKindToTransactionKind(&kind.TransactionKind)
	}
	return kinds
}
