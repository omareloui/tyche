package repository

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/omareloui/tyche/internal/dbtypes"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

func (r *Repository) GetWishlistItems(ctx context.Context) ([]models.WishlistItem, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := q.GetWishlistItems(ctx)
	if err != nil {
		l.Error("fetching wishlist items", zap.Error(err))
		return nil, err
	}

	items := make([]models.WishlistItem, len(rows))
	for i, row := range rows {
		items[i] = *dbWishlistItemToWishlistItem(&row.WishlistItem, &row.Currency)
	}
	return items, nil
}

func (r *Repository) GetWishlistItem(ctx context.Context, id string) (*models.WishlistItem, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	row, err := q.GetWishlistItem(ctx, id)
	if err != nil {
		return nil, err
	}
	return dbWishlistItemToWishlistItem(&row.WishlistItem, &row.Currency), nil
}

func (r *Repository) CreateWishlistItem(ctx context.Context, w *models.WishlistItem) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	plan, err := dbWishlistPlan(w.Plan)
	if err != nil {
		return err
	}

	return q.CreateWishlistItem(ctx, queries.CreateWishlistItemParams{
		ID:                w.ID,
		Name:              w.Name,
		Price:             w.Price,
		CurrencyID:        w.Currency.ID,
		Installments:      plan.months,
		InstallmentAmount: plan.amount,
		InstallmentPlan:   plan.json,
		Url:               w.URL,
		Note:              w.Note,
		CoolingDays:       int64(w.CoolingDays),
		CreatedAt:         w.CreatedAt,
	})
}

func (r *Repository) UpdateWishlistItem(ctx context.Context, w *models.WishlistItem) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	plan, err := dbWishlistPlan(w.Plan)
	if err != nil {
		return err
	}

	return q.UpdateWishlistItem(ctx, queries.UpdateWishlistItemParams{
		ID:                w.ID,
		Name:              w.Name,
		Price:             w.Price,
		CurrencyID:        w.Currency.ID,
		Installments:      plan.months,
		InstallmentAmount: plan.amount,
		InstallmentPlan:   plan.json,
		Url:               w.URL,
		Note:              w.Note,
		CoolingDays:       int64(w.CoolingDays),
	})
}

// SetWishlistItemStatus decides on an item (or puts it back to waiting),
// linking the transaction it was bought with, if any.
func (r *Repository) SetWishlistItemStatus(ctx context.Context, id string, status models.WishlistStatus, txID string) error {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	if err := q.SetWishlistItemStatus(ctx, queries.SetWishlistItemStatusParams{
		ID:            id,
		Status:        int64(status),
		TransactionID: sql.NullString{String: txID, Valid: txID != ""},
	}); err != nil {
		l.Error("setting wishlist item status", zap.String("id", id), zap.Error(err))
		return err
	}
	return nil
}

func (r *Repository) DeleteWishlistItem(ctx context.Context, id string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	return q.DeleteWishlistItem(ctx, id)
}

func dbWishlistItemToWishlistItem(w *queries.WishlistItem, cur *queries.Currency) *models.WishlistItem {
	return &models.WishlistItem{
		ID:            w.ID,
		Name:          w.Name,
		Price:         w.Price,
		Currency:      *dbCurrencyToCurrency(cur, false),
		Plan:          wishlistPlanFromDB(w),
		URL:           w.Url,
		Note:          w.Note,
		CoolingDays:   int(w.CoolingDays),
		Status:        models.WishlistStatus(w.Status),
		DecidedAt:     w.DecidedAt.Time,
		TransactionID: w.TransactionID.String,
		CreatedAt:     w.CreatedAt,
	}
}

type wishlistPlanColumns struct {
	months int64
	amount float64
	json   dbtypes.JSON
}

// dbWishlistPlan is the plan's columns: the whole plan as JSON, plus its
// months and monthly amount on their own.
func dbWishlistPlan(plan *models.InstallmentPlan) (wishlistPlanColumns, error) {
	if plan == nil {
		return wishlistPlanColumns{}, nil
	}
	b, err := json.Marshal(plan)
	if err != nil {
		return wishlistPlanColumns{}, err
	}
	return wishlistPlanColumns{months: int64(plan.Months), amount: plan.Amount, json: b}, nil
}

// wishlistPlanFromDB reads the item's plan, items from before plans being a
// known amount for their months.
func wishlistPlanFromDB(w *queries.WishlistItem) *models.InstallmentPlan {
	if len(w.InstallmentPlan) > 0 {
		var plan models.InstallmentPlan
		if err := json.Unmarshal(w.InstallmentPlan, &plan); err == nil {
			return &plan
		}
	}
	if w.Installments > 0 {
		return &models.InstallmentPlan{Months: int(w.Installments), Amount: w.InstallmentAmount}
	}
	return nil
}
