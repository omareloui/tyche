package repository

import (
	"context"
	"database/sql"

	"github.com/omareloui/tyche/internal/queries"
)

// The Set*Note methods set a note alone, from the details' quick edit.

func (r *Repository) SetTransactionNote(ctx context.Context, id, note string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.SetTransactionNote(ctx, queries.SetTransactionNoteParams{ID: id, Note: note})
}

// SetRecurrentTxNote sets the recurrent transaction's note, and its scheduled
// occurrences' but the ones edited on their own.
func (r *Repository) SetRecurrentTxNote(ctx context.Context, id, note string) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		if err := qtx.SetRecurrentTxNote(ctx, queries.SetRecurrentTxNoteParams{ID: id, Note: note}); err != nil {
			return err
		}
		return qtx.SetUnfulfilledOccurrencesNote(ctx, queries.SetUnfulfilledOccurrencesNoteParams{
			RecurrentID: sql.NullString{String: id, Valid: true}, Note: note,
		})
	})
}

func (r *Repository) SetAccountNote(ctx context.Context, id, note string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.SetAccountNote(ctx, queries.SetAccountNoteParams{ID: id, Note: sql.NullString{String: note, Valid: note != ""}})
}

func (r *Repository) SetWishlistItemNote(ctx context.Context, id, note string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.SetWishlistItemNote(ctx, queries.SetWishlistItemNoteParams{ID: id, Note: note})
}

func (r *Repository) SetCounterpartyNote(ctx context.Context, id, note string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.SetCounterpartyNote(ctx, queries.SetCounterpartyNoteParams{ID: id, Note: note})
}
