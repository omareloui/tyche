package repository

import (
	"context"
	"database/sql"

	"github.com/omareloui/tyche/internal/queries"
)

// The setters of the details' quick picks: one reference alone, empty
// clearing it.

// SetCounterpartyDefaultCategory sets a merchant's default category (left
// alone on other counterparties, which take none).
func (r *Repository) SetCounterpartyDefaultCategory(ctx context.Context, id, categoryID string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.SetCounterpartyDefaultCategory(ctx, queries.SetCounterpartyDefaultCategoryParams{
		ID: id, DefaultCategoryID: sql.NullString{String: categoryID, Valid: categoryID != ""},
	})
}

// SetItemCategory sets an item's category (an item category).
func (r *Repository) SetItemCategory(ctx context.Context, id, categoryID string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.SetItemCategory(ctx, queries.SetItemCategoryParams{
		ID: id, CategoryID: sql.NullString{String: categoryID, Valid: categoryID != ""},
	})
}
