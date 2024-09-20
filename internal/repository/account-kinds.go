package repository

import (
	"context"

	"github.com/omareloui/financial-planner/internal/db"
	"github.com/omareloui/financial-planner/internal/logger"
	"github.com/omareloui/financial-planner/internal/models"
	"go.uber.org/zap"
)

func (r *Repository) GetAccountKinds(ctx context.Context) ([]models.AccountKind, error) {
	l := logger.FromCtx(ctx)

	dbKnds, err := r.q.GetAccountKinds(ctx)
	if err != nil {
		l.Error("querying account kinds", zap.Error(err))
		return nil, err
	}
	l.Debug("queried account kinds")

	return r.dbAccountKindsToAccountKinds(dbKnds), nil
}

func (r *Repository) AccountKindsToMap(knds []models.AccountKind) map[string]string {
	m := make(map[string]string, len(knds))
	for _, k := range knds {
		m[k.ID] = k.Enum.String()
	}
	return m
}

func (r *Repository) dbAccountKindToAccountKind(dbAk *db.AccountKind) *models.AccountKind {
	return &models.AccountKind{
		ID:   dbAk.ID,
		Enum: models.AccountKindEnum(dbAk.Enum),
	}
}

func (r *Repository) dbAccountKindsToAccountKinds(dbAks []db.AccountKind) []models.AccountKind {
	ak := make([]models.AccountKind, len(dbAks))
	for i, dbAk := range dbAks {
		ak[i] = *r.dbAccountKindToAccountKind(&dbAk)
	}
	return ak
}
