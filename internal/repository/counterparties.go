package repository

import (
	"context"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

func (r *Repository) GetCounterparties(ctx context.Context) ([]models.Counterparty, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		l.Error("getting db from context", zap.Error(err))
		return nil, err
	}

	dbCp, err := q.GetCounterparties(ctx)
	if err != nil {
		l.Error("fetching counter parties", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched counter parties")
	return dbCounterpartiesRowToCounterparties(dbCp), nil
}

func (r *Repository) GetCounterparty(ctx context.Context, id string) (*models.Counterparty, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		l.Error("getting db from context", zap.Error(err))
		return nil, err
	}

	dbP, err := q.GetCounterparty(ctx, id)
	if err != nil {
		return nil, err
	}
	return dbCounterpartyToCounterparty(&dbP.Counterparty), nil
}

func (r *Repository) UpdateCounterparty(ctx context.Context, it *models.Counterparty) error {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		l.Error("getting db from context", zap.Error(err))
		return err
	}

	return q.UpdateCounterparty(ctx, queries.UpdateCounterpartyParams{
		ID:   it.ID,
		Name: it.Name,
	})
}

func (r *Repository) CreateCounterparty(ctx context.Context, it *models.Counterparty) error {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		l.Error("getting db from context", zap.Error(err))
		return err
	}

	return q.CreateCounterparty(ctx, queries.CreateCounterpartyParams{
		ID:   it.ID,
		Name: it.Name,
	})
}

func (r *Repository) DeleteCounterparty(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		l.Error("getting db from context", zap.Error(err))
		return err
	}

	return q.DeleteCounterparty(ctx, id)
}

func dbCounterpartyToCounterparty(dbP *queries.Counterparty) *models.Counterparty {
	return &models.Counterparty{
		ID:        dbP.ID,
		Name:      dbP.Name,
		CreatedAt: dbP.CreatedAt,
	}
}

func dbCounterpartiesRowToCounterparties(dbCps []queries.GetCounterpartiesRow) []models.Counterparty {
	cps := make([]models.Counterparty, len(dbCps))
	for i, cp := range dbCps {
		cps[i] = *dbCounterpartyToCounterparty(&cp.Counterparty)
	}
	return cps
}
