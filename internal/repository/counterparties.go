package repository

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/omareloui/tyche/internal/brandicons"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

func (r *Repository) GetCounterparties(ctx context.Context) ([]models.Counterparty, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbCp, err := q.GetCounterparties(ctx)
	if err != nil {
		l.Error("fetching counter parties", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched counter parties")

	// Backfill counterparties created before icon detection.
	icons := brandicons.Default()
	for i, cp := range dbCp {
		dbIt := &dbCp[i].Counterparty
		if dbIt.Icon.Valid && dbIt.Color.Valid {
			continue
		}
		if !dbIt.Icon.Valid {
			aliases, _ := unmarshalAliases(dbIt.Aliases)
			dbIt.Icon = sql.NullString{String: icons.Detect(dbIt.Name, aliases), Valid: true}
		}
		dbIt.Color = sql.NullString{String: icons.ColorFor(dbIt.Icon.String, dbIt.Color.String), Valid: true}
		if err := q.SetCounterpartyAppearance(ctx, queries.SetCounterpartyAppearanceParams{ID: cp.Counterparty.ID, Icon: dbIt.Icon, Color: dbIt.Color}); err != nil {
			l.Error("backfilling counterparty icon", zap.Error(err), zap.String("id", cp.Counterparty.ID))
		}
	}

	return dbCounterpartiesRowToCounterparties(dbCp), nil
}

func (r *Repository) GetCounterparty(ctx context.Context, id string) (*models.Counterparty, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbP, err := q.GetCounterparty(ctx, id)
	if err != nil {
		return nil, err
	}
	return dbCounterpartyToCounterparty(&dbP.Counterparty), nil
}

func (r *Repository) UpdateCounterparty(ctx context.Context, it *models.Counterparty) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	aliases, err := marshalAliases(it.Aliases)
	if err != nil {
		return err
	}

	return q.UpdateCounterparty(ctx, queries.UpdateCounterpartyParams{
		ID:      it.ID,
		Name:    it.Name,
		Aliases: aliases,
		Icon:    sql.NullString{String: it.Icon, Valid: true},
		Color:   sql.NullString{String: it.Color, Valid: it.Color != ""},
	})
}

func (r *Repository) CreateCounterparty(ctx context.Context, it *models.Counterparty) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	aliases, err := marshalAliases(it.Aliases)
	if err != nil {
		return err
	}

	return q.CreateCounterparty(ctx, queries.CreateCounterpartyParams{
		ID:      it.ID,
		Name:    it.Name,
		Aliases: aliases,
		Icon:    sql.NullString{String: it.Icon, Valid: true},
		Color:   sql.NullString{String: it.Color, Valid: it.Color != ""},
	})
}

func (r *Repository) DeleteCounterparty(ctx context.Context, id string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	return q.DeleteCounterparty(ctx, id)
}

func dbCounterpartyToCounterparty(dbP *queries.Counterparty) *models.Counterparty {
	aliases, _ := unmarshalAliases(dbP.Aliases)
	return &models.Counterparty{
		ID:        dbP.ID,
		Name:      dbP.Name,
		Aliases:   aliases,
		Icon:      dbP.Icon.String,
		Color:     dbP.Color.String,
		CreatedAt: dbP.CreatedAt,
	}
}

func marshalAliases(aliases []string) ([]byte, error) {
	if aliases == nil {
		aliases = []string{}
	}
	marshaled, err := json.Marshal(aliases)
	if err != nil {
		logger.Get().Error("error on marshaling counterparty aliases for db", zap.Error(err))
		return nil, err
	}
	return marshaled, nil
}

func unmarshalAliases(dbAliases []byte) ([]string, error) {
	if dbAliases == nil {
		return []string{}, nil
	}

	aliases := []string{}
	if err := json.Unmarshal(dbAliases, &aliases); err != nil {
		logger.Get().Error("error on unmarshaling counterparty aliases from db", zap.Error(err))
		return []string{}, err
	}
	return aliases, nil
}

func dbCounterpartiesRowToCounterparties(dbCps []queries.GetCounterpartiesRow) []models.Counterparty {
	cps := make([]models.Counterparty, len(dbCps))
	for i, cp := range dbCps {
		cps[i] = *dbCounterpartyToCounterparty(&cp.Counterparty)
	}
	return cps
}
