package repository

import (
	"context"

	"github.com/omareloui/tyche/internal/db"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"go.uber.org/zap"
)

func (r *Repository) GetCounterparties(ctx context.Context) ([]models.Counterparty, error) {
	l := logger.FromCtx(ctx)
	dbCp, err := r.q.GetCounterparties(ctx)
	if err != nil {
		l.Error("fetching counter parties", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched counter parties")
	return dbCounterpartiesRowToCounterparties(dbCp), nil
}

func (r *Repository) GetCounterparty(ctx context.Context, id string) (*models.Counterparty, error) {
	dbP, err := r.q.GetCounterparty(ctx, id)
	if err != nil {
		return nil, err
	}
	return dbCounterpartyRowToCounterparty(&dbP), nil
}

func (r *Repository) UpdateCounterparty(ctx context.Context, it *models.Counterparty) error {
	return r.q.UpdateCounterparty(ctx, db.UpdateCounterpartyParams{
		ID:   it.ID,
		Name: it.Name,
	})
}

func (r *Repository) CreateCounterparty(ctx context.Context, it *models.Counterparty) error {
	return r.q.CreateCounterparty(ctx, db.CreateCounterpartyParams{
		ID:   it.ID,
		Name: it.Name,
	})
}

func (r *Repository) DeleteCounterparty(ctx context.Context, id string) error {
	return r.q.DeleteCounterparty(ctx, id)
}

func dbCounterpartyRowToCounterparty(dbP *db.Counterparty) *models.Counterparty {
	return &models.Counterparty{
		ID:        dbP.ID,
		Name:      dbP.Name,
		CreatedAt: dbP.CreatedAt,
	}
}

func dbCounterpartiesRowToCounterparties(dbCps []db.Counterparty) []models.Counterparty {
	cps := make([]models.Counterparty, len(dbCps))
	for i, cp := range dbCps {
		cps[i] = models.Counterparty{
			ID:        cp.ID,
			Name:      cp.Name,
			CreatedAt: cp.CreatedAt,
		}
	}
	return cps
}
