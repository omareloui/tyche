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

func (r *Repository) createAccountKind(ctx context.Context, knd models.AccountKind) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	l := logger.FromCtx(ctx)

	if err := q.CreateAccountKind(ctx, queries.CreateAccountKindParams{
		ID:   knd.ID,
		Enum: int64(knd.Enum),
	}); err != nil {
		l.Error("error creating account kind", zap.Any("kind", knd), zap.Error(err))
		return err
	}

	l.Debug("created account kind", zap.Any("kind", knd))
	return nil
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
