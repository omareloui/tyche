package repository

import (
	"context"
	"database/sql"

	"github.com/omareloui/tyche/internal/brandicons"
	"github.com/omareloui/tyche/internal/idgen"
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
				aliases, _ := unmarshalStrings(dbIt.Aliases)
				icon = icons.Detect(dbIt.Name, aliases)
			}
			dbIt.Icon = sql.NullString{String: icon, Valid: true}
		}
		dbIt.Color = sql.NullString{String: icons.ColorFor(dbIt.Icon.String, dbIt.Color.String), Valid: true}
		if err := q.SetCounterpartyAppearance(ctx, queries.SetCounterpartyAppearanceParams{ID: cp.Counterparty.ID, Icon: dbIt.Icon, Color: dbIt.Color}); err != nil {
			l.Error("backfilling counterparty icon", zap.Error(err), zap.String("id", cp.Counterparty.ID))
		}
	}

	cps := dbCounterpartiesRowToCounterparties(dbCp)

	dbLocs, err := q.GetCounterpartyLocations(ctx)
	if err != nil {
		l.Error("fetching counterparty locations", zap.Error(err))
		return nil, err
	}
	locs := map[string][]models.CounterpartyLocation{}
	for _, loc := range dbLocs {
		locs[loc.CounterpartyID] = append(locs[loc.CounterpartyID], dbLocationToLocation(loc))
	}
	for i := range cps {
		cps[i].Locations = locs[cps[i].ID]
	}

	return cps, nil
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
	cp := dbCounterpartyRowToCounterparty(queries.GetCounterpartiesRow(dbP))

	dbLocs, err := q.GetCounterpartyLocationsOf(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, loc := range dbLocs {
		cp.Locations = append(cp.Locations, dbLocationToLocation(loc))
	}
	return cp, nil
}

// UpdateCounterparty saves the counterparty and replaces its locations.
func (r *Repository) UpdateCounterparty(ctx context.Context, it *models.Counterparty) error {
	params, err := updateCounterpartyParams(it)
	if err != nil {
		return err
	}
	return r.transact(ctx, func(qtx *queries.Queries) error {
		if err := qtx.UpdateCounterparty(ctx, params); err != nil {
			return err
		}
		return replaceCounterpartyLocations(ctx, qtx, it)
	})
}

// replaceCounterpartyLocations writes the counterparty's locations over its
// saved ones, giving new ones an id.
func replaceCounterpartyLocations(ctx context.Context, qtx *queries.Queries, it *models.Counterparty) error {
	if err := qtx.DeleteCounterpartyLocations(ctx, it.ID); err != nil {
		return err
	}
	for i := range it.Locations {
		loc := &it.Locations[i]
		if loc.ID == "" {
			loc.ID = idgen.New()
		}
		if err := qtx.CreateCounterpartyLocation(ctx, queries.CreateCounterpartyLocationParams{
			ID: loc.ID, CounterpartyID: it.ID, Label: loc.Label, Address: loc.Address,
			MapsUrl: loc.MapsURL, Position: int64(i),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) SetCounterpartyRating(ctx context.Context, id string, rating int) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	return q.SetCounterpartyRating(ctx, queries.SetCounterpartyRatingParams{ID: id, Rating: ratingParam(rating)})
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
		intoLocs, err := qtx.GetCounterpartyLocationsOf(ctx, intoID)
		if err != nil {
			return err
		}
		if err := qtx.ReassignCounterpartyLocations(ctx, queries.ReassignCounterpartyLocationsParams{
			ToID: intoID, FromID: fromID, Offset: int64(len(intoLocs)),
		}); err != nil {
			return err
		}
		merged, err := qtx.GetCounterpartyLocationsOf(ctx, intoID)
		if err != nil {
			return err
		}
		for _, loc := range merged {
			into.Locations = append(into.Locations, dbLocationToLocation(loc))
		}

		return qtx.DeleteCounterparty(ctx, fromID)
	})
	if err != nil {
		return nil, err
	}
	return into, nil
}

func updateCounterpartyParams(it *models.Counterparty) (queries.UpdateCounterpartyParams, error) {
	aliases, err := marshalStrings(it.Aliases)
	if err != nil {
		return queries.UpdateCounterpartyParams{}, err
	}
	labels, err := marshalStrings(it.Labels)
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
		IsArchived:        it.IsArchived,
		Rating:            ratingParam(it.Rating),
		Note:              it.Note,
		Labels:            labels,
	}, nil
}

// CreateCounterparty saves the counterparty with its locations.
func (r *Repository) CreateCounterparty(ctx context.Context, it *models.Counterparty) error {
	aliases, err := marshalStrings(it.Aliases)
	if err != nil {
		return err
	}
	labels, err := marshalStrings(it.Labels)
	if err != nil {
		return err
	}

	return r.transact(ctx, func(qtx *queries.Queries) error {
		if err := qtx.CreateCounterparty(ctx, queries.CreateCounterpartyParams{
			ID:                it.ID,
			Name:              it.Name,
			Aliases:           aliases,
			Icon:              sql.NullString{String: it.Icon, Valid: true},
			Color:             sql.NullString{String: it.Color, Valid: it.Color != ""},
			IsMerchant:        it.IsMerchant,
			DefaultCategoryID: defaultCategoryID(it),
			Rating:            ratingParam(it.Rating),
			Note:              it.Note,
			Labels:            labels,
		}); err != nil {
			return err
		}
		return replaceCounterpartyLocations(ctx, qtx, it)
	})
}

func (r *Repository) SetCounterpartyArchived(ctx context.Context, id string, archived bool) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	return q.SetCounterpartyArchived(ctx, queries.SetCounterpartyArchivedParams{ID: id, IsArchived: archived})
}

func (r *Repository) DeleteCounterparty(ctx context.Context, id string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	return q.DeleteCounterparty(ctx, id)
}

func dbCounterpartyToCounterparty(dbP *queries.Counterparty) *models.Counterparty {
	aliases, _ := unmarshalStrings(dbP.Aliases)
	labels, _ := unmarshalStrings(dbP.Labels)
	return &models.Counterparty{
		ID:         dbP.ID,
		Name:       dbP.Name,
		Aliases:    aliases,
		Icon:       dbP.Icon.String,
		Color:      dbP.Color.String,
		IsMerchant: dbP.IsMerchant,
		IsArchived: dbP.IsArchived,
		Rating:     int(dbP.Rating.Int64),
		Note:       dbP.Note,
		Labels:     labels,
		CreatedAt:  dbP.CreatedAt,
	}
}

func dbLocationToLocation(loc queries.CounterpartyLocation) models.CounterpartyLocation {
	return models.CounterpartyLocation{ID: loc.ID, Label: loc.Label, Address: loc.Address, MapsURL: loc.MapsUrl}
}

// ratingParam is NULL for an unrated (0) counterparty.
func ratingParam(rating int) sql.NullInt64 {
	return sql.NullInt64{Int64: int64(rating), Valid: rating > 0}
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

func dbCounterpartiesRowToCounterparties(dbCps []queries.GetCounterpartiesRow) []models.Counterparty {
	cps := make([]models.Counterparty, len(dbCps))
	for i, cp := range dbCps {
		cps[i] = *dbCounterpartyRowToCounterparty(cp)
	}
	return cps
}

// GetCounterpartyRecentTransactions is the counterparty's last n fulfilled
// transactions, newest first.
func (r *Repository) GetCounterpartyRecentTransactions(ctx context.Context, id string, n int) ([]models.Transaction, error) {
	p := txFilterParams(&models.TxFilter{CounterpartyID: id})
	p.Fulfilled = true
	p.Lim = int64(n)
	return r.listTransactions(ctx, p)
}

// AddCounterpartyLocation appends a location to the counterparty's, giving it
// an id.
func (r *Repository) AddCounterpartyLocation(ctx context.Context, cpID string, loc *models.CounterpartyLocation) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		locs, err := qtx.GetCounterpartyLocationsOf(ctx, cpID)
		if err != nil {
			return err
		}
		loc.ID = idgen.New()
		return qtx.CreateCounterpartyLocation(ctx, queries.CreateCounterpartyLocationParams{
			ID: loc.ID, CounterpartyID: cpID, Label: loc.Label, Address: loc.Address,
			MapsUrl: loc.MapsURL, Position: int64(len(locs)),
		})
	})
}

func (r *Repository) SetCounterpartyLabels(ctx context.Context, id string, labels []string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	dbLabels, err := marshalStrings(labels)
	if err != nil {
		return err
	}
	return q.SetCounterpartyLabels(ctx, queries.SetCounterpartyLabelsParams{ID: id, Labels: dbLabels})
}
