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
			icon := ""
			if dbIt.IsMerchant {
				aliases, _ := unmarshalAliases(dbIt.Aliases)
				icon = icons.Detect(dbIt.Name, aliases)
			}
			dbIt.Icon = sql.NullString{String: icon, Valid: true}
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
	return dbCounterpartyRowToCounterparty(queries.GetCounterpartiesRow(dbP)), nil
}

func (r *Repository) UpdateCounterparty(ctx context.Context, it *models.Counterparty) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	params, err := updateCounterpartyParams(it)
	if err != nil {
		return err
	}
	return q.UpdateCounterparty(ctx, params)
}

// MergeCounterparties moves everything referencing the from counterparty to
// the into one, adds its name and aliases to the into aliases, then deletes it.
func (r *Repository) MergeCounterparties(ctx context.Context, fromID, intoID string) (*models.Counterparty, error) {
	var into *models.Counterparty
	err := r.transact(ctx, func(qtx *queries.Queries) error {
		dbFrom, err := qtx.GetCounterparty(ctx, fromID)
		if err != nil {
			return err
		}
		dbInto, err := qtx.GetCounterparty(ctx, intoID)
		if err != nil {
			return err
		}
		into = dbCounterpartyRowToCounterparty(queries.GetCounterpartiesRow(dbInto))
		into.Absorb(dbCounterpartyRowToCounterparty(queries.GetCounterpartiesRow(dbFrom)))

		// Detect the icon if not yet detected, as the absorbed aliases may match.
		if into.IsMerchant && !dbInto.Counterparty.Icon.Valid {
			icons := brandicons.Default()
			into.Icon = icons.Detect(into.Name, into.Aliases)
			into.Color = icons.ColorFor(into.Icon, into.Color)
		}

		params, err := updateCounterpartyParams(into)
		if err != nil {
			return err
		}
		if err := qtx.UpdateCounterparty(ctx, params); err != nil {
			return err
		}

		from := sql.NullString{String: fromID, Valid: true}
		to := sql.NullString{String: intoID, Valid: true}
		if err := qtx.ReassignTransactionsCounterparty(ctx, queries.ReassignTransactionsCounterpartyParams{ToID: to, FromID: from}); err != nil {
			return err
		}
		if err := qtx.ReassignRecurrentTxsCounterparty(ctx, queries.ReassignRecurrentTxsCounterpartyParams{ToID: to, FromID: from}); err != nil {
			return err
		}
		if err := qtx.ReassignAccountsCounterparty(ctx, queries.ReassignAccountsCounterpartyParams{ToID: to, FromID: from}); err != nil {
			return err
		}

		return qtx.DeleteCounterparty(ctx, fromID)
	})
	if err != nil {
		return nil, err
	}
	return into, nil
}

func updateCounterpartyParams(it *models.Counterparty) (queries.UpdateCounterpartyParams, error) {
	aliases, err := marshalAliases(it.Aliases)
	if err != nil {
		return queries.UpdateCounterpartyParams{}, err
	}
	return queries.UpdateCounterpartyParams{
		ID:                it.ID,
		Name:              it.Name,
		Aliases:           aliases,
		Icon:              sql.NullString{String: it.Icon, Valid: true},
		Color:             sql.NullString{String: it.Color, Valid: it.Color != ""},
		IsMerchant:        it.IsMerchant,
		DefaultCategoryID: defaultCategoryID(it),
	}, nil
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
		ID:                it.ID,
		Name:              it.Name,
		Aliases:           aliases,
		Icon:              sql.NullString{String: it.Icon, Valid: true},
		Color:             sql.NullString{String: it.Color, Valid: it.Color != ""},
		IsMerchant:        it.IsMerchant,
		DefaultCategoryID: defaultCategoryID(it),
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
		ID:         dbP.ID,
		Name:       dbP.Name,
		Aliases:    aliases,
		Icon:       dbP.Icon.String,
		Color:      dbP.Color.String,
		IsMerchant: dbP.IsMerchant,
		CreatedAt:  dbP.CreatedAt,
	}
}

func dbCounterpartyRowToCounterparty(dbP queries.GetCounterpartiesRow) *models.Counterparty {
	cp := dbCounterpartyToCounterparty(&dbP.Counterparty)
	if dbP.Counterparty.DefaultCategoryID.Valid {
		kinds, _ := unmarshalTxKinds(dbP.DefaultCategoryKinds)
		cp.DefaultCategory = &models.Category{
			ID:    dbP.Counterparty.DefaultCategoryID.String,
			Name:  dbP.DefaultCategoryName.String,
			Color: dbP.DefaultCategoryColor.String,
			Kinds: kinds,
		}
	}
	return cp
}

func defaultCategoryID(cp *models.Counterparty) sql.NullString {
	if cp.DefaultCategory == nil || cp.DefaultCategory.ID == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: cp.DefaultCategory.ID, Valid: true}
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
		cps[i] = *dbCounterpartyRowToCounterparty(cp)
	}
	return cps
}
