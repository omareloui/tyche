package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
)

// MerchantGuessWrite is an AI merchant decision to save: Raw added as an
// alias of the merchant CounterpartyID, or New created (Raw among its
// aliases).
type MerchantGuessWrite struct {
	Raw            string
	CounterpartyID string
	New            *models.Counterparty
}

// GetMerchantGuesses are every AI merchant decision, rejected ones too.
func (r *Repository) GetMerchantGuesses(ctx context.Context) ([]models.MerchantGuess, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.GetMerchantGuesses(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]models.MerchantGuess, len(rows))
	for i := range rows {
		out[i] = dbMerchantGuess(rows[i])
	}
	return out, nil
}

// SaveMerchantGuesses writes the decisions in one transaction: aliases
// added, merchants created, each kept as a MerchantGuess. A decision whose
// merchant is gone is left out. Answers the guesses saved.
func (r *Repository) SaveMerchantGuesses(ctx context.Context, writes []MerchantGuessWrite) ([]models.MerchantGuess, error) {
	var saved []models.MerchantGuess
	err := r.transact(ctx, func(qtx *queries.Queries) error {
		saved = nil
		for _, w := range writes {
			g := models.MerchantGuess{ID: idgen.New(), Raw: w.Raw, CounterpartyID: w.CounterpartyID}
			if w.New != nil {
				if err := createCounterpartyWithTxn(ctx, qtx, w.New); err != nil {
					return err
				}
				g.CounterpartyID, g.Created = w.New.ID, true
				if w.New.DefaultCategory != nil {
					g.CategoryID = w.New.DefaultCategory.ID
				}
			} else {
				row, err := qtx.GetCounterparty(ctx, w.CounterpartyID)
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				if err != nil {
					return err
				}
				aliases, _ := unmarshalStrings(row.Counterparty.Aliases)
				dbAliases, err := marshalStrings(models.WithAlias(row.Counterparty.Name, aliases, w.Raw))
				if err != nil {
					return err
				}
				if err := qtx.SetCounterpartyAliases(ctx, queries.SetCounterpartyAliasesParams{ID: w.CounterpartyID, Aliases: dbAliases}); err != nil {
					return err
				}
			}
			if err := qtx.CreateMerchantGuess(ctx, queries.CreateMerchantGuessParams{
				ID: g.ID, Raw: g.Raw, CounterpartyID: g.CounterpartyID,
				CategoryID: sql.NullString{String: g.CategoryID, Valid: g.CategoryID != ""}, Created: g.Created,
			}); err != nil {
				return err
			}
			saved = append(saved, g)
		}
		return nil
	})
	return saved, err
}

// GetTxMerchantGuess is the guess giving the transaction its merchant, nil
// when none (or rejected since).
func (r *Repository) GetTxMerchantGuess(ctx context.Context, txID string) (*models.MerchantGuess, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	row, err := q.GetTransactionMerchantGuess(ctx, txID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g := dbMerchantGuess(row)
	return &g, nil
}

// RejectMerchantGuess undoes a guess: its string is no longer an alias of
// its merchant (which stays), no transaction is marked by it and the string
// is never asked about again.
func (r *Repository) RejectMerchantGuess(ctx context.Context, id string) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		g, err := qtx.GetMerchantGuess(ctx, id)
		if err != nil {
			return err
		}
		row, err := qtx.GetCounterparty(ctx, g.CounterpartyID)
		if err == nil {
			aliases, _ := unmarshalStrings(row.Counterparty.Aliases)
			dbAliases, err := marshalStrings(models.WithoutAlias(aliases, g.Raw))
			if err != nil {
				return err
			}
			if err := qtx.SetCounterpartyAliases(ctx, queries.SetCounterpartyAliasesParams{ID: g.CounterpartyID, Aliases: dbAliases}); err != nil {
				return err
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := qtx.RejectMerchantGuess(ctx, id); err != nil {
			return err
		}
		return qtx.ClearTransactionsMerchantGuess(ctx, sql.NullString{String: id, Valid: true})
	})
}

// GetAIMerchants tells the user turned matching merchants with AI on (off
// without a user row yet).
func (r *Repository) GetAIMerchants(ctx context.Context) (bool, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return false, err
	}
	on, err := q.GetAIMerchants(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return on, err
}

// SetAIMerchants turns matching merchants with AI on or off.
func (r *Repository) SetAIMerchants(ctx context.Context, on bool) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.SetAIMerchants(ctx, on)
}

func dbMerchantGuess(g queries.MerchantGuess) models.MerchantGuess {
	out := models.MerchantGuess{
		ID: g.ID, Raw: g.Raw, CounterpartyID: g.CounterpartyID, CategoryID: g.CategoryID.String,
		Created: g.Created, CreatedAt: g.CreatedAt,
	}
	if g.RejectedAt.Valid {
		t := g.RejectedAt.Time
		out.RejectedAt = &t
	}
	return out
}
