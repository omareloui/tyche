package repository

import (
	"context"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

func (r *Repository) GetAccountKinds(ctx context.Context) ([]models.AccountKind, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		l.Error("getting db from context", zap.Error(err))
		return nil, err
	}

	dbKnds, err := q.GetAccountKinds(ctx)
	if err != nil {
		l.Error("querying account kinds", zap.Error(err))
		return nil, err
	}
	l.Debug("queried account kinds")

	return dbAccountKindsToAccountKinds(dbKnds), nil
}

func (r *Repository) AccountKindsToMap(knds []models.AccountKind) map[string]string {
	m := make(map[string]string, len(knds))
	for _, k := range knds {
		m[k.ID] = k.Enum.String()
	}
	return m
}

func dbAccountKindToAccountKind(dbAk *queries.AccountKind) *models.AccountKind {
	return &models.AccountKind{
		ID:   dbAk.ID,
		Enum: models.AccKind(dbAk.Enum),
	}
}

func dbAccountKindsToAccountKinds(dbAks []queries.GetAccountKindsRow) []models.AccountKind {
	ak := make([]models.AccountKind, len(dbAks))
	for i, dbAk := range dbAks {
		ak[i] = *dbAccountKindToAccountKind(&dbAk.AccountKind)
	}
	return ak
}
