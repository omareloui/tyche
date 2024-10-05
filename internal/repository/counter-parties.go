package repository

import (
	"context"

	"github.com/omareloui/tyche/internal/db"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"go.uber.org/zap"
)

func (r *Repository) GetCounterParties(ctx context.Context) ([]models.CounterParty, error) {
	l := logger.FromCtx(ctx)
	dbCp, err := r.q.GetCounterParties(ctx)
	if err != nil {
		l.Error("fetching counter parties", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched counter parties")
	return dbCounterPartiesRowToCounterParties(dbCp), nil
}

func (r *Repository) GetCounterParty(ctx context.Context, id string) (*models.CounterParty, error) {
	dbP, err := r.q.GetCounterParty(ctx, id)
	if err != nil {
		return nil, err
	}
	return dbCounterPartyRowToCounterParty(&dbP), nil
}

func (r *Repository) UpdateCounterParty(ctx context.Context, it *models.CounterParty) error {
	return r.q.UpdateCounterParty(ctx, db.UpdateCounterPartyParams{
		ID:   it.ID,
		Name: it.Name,
	})
}

func (r *Repository) CreateCounterParty(ctx context.Context, it *models.CounterParty) error {
	return r.q.CreateCounterParty(ctx, db.CreateCounterPartyParams{
		ID:   it.ID,
		Name: it.Name,
	})
}

func (r *Repository) DeleteCounterParty(ctx context.Context, id string) error {
	return r.q.DeleteCounterParty(ctx, id)
}

func dbCounterPartyRowToCounterParty(dbP *db.CounterParty) *models.CounterParty {
	return &models.CounterParty{
		ID:        dbP.ID,
		Name:      dbP.Name,
		CreatedAt: dbP.CreatedAt,
	}
}

func dbCounterPartiesRowToCounterParties(dbCps []db.CounterParty) []models.CounterParty {
	cps := make([]models.CounterParty, len(dbCps))
	for i, cp := range dbCps {
		cps[i] = models.CounterParty{
			ID:        cp.ID,
			Name:      cp.Name,
			CreatedAt: cp.CreatedAt,
		}
	}
	return cps
}
