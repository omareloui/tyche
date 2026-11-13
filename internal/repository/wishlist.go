package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

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

// GetWishlistItemByGoal is the wish the goal saves for, nil when none.
func (r *Repository) GetWishlistItemByGoal(ctx context.Context, goalID string) (*models.WishlistItem, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	row, err := q.GetWishlistItemByGoal(ctx, sql.NullString{String: goalID, Valid: true})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
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
		GoalID:        w.GoalID.String,
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

// setWishlistItemStatusByGoalWithTxn decides the wish the goal saves for, if
// any, linking the transaction it was bought with.
func setWishlistItemStatusByGoalWithTxn(ctx context.Context, qtx *queries.Queries, goalID string, status models.WishlistStatus, txID string) error {
	if err := qtx.SetWishlistItemStatusByGoal(ctx, queries.SetWishlistItemStatusByGoalParams{
		GoalID:        sql.NullString{String: goalID, Valid: true},
		Status:        int64(status),
		TransactionID: sql.NullString{String: txID, Valid: txID != ""},
	}); err != nil {
		logger.FromCtx(ctx).Error("deciding the goal's wish", zap.String("goal", goalID), zap.Error(err))
		return err
	}
	return nil
}

var (
	// ErrWishNotWaiting is a wish already decided, which can't be bought in a
	// transaction.
	ErrWishNotWaiting = errors.New("the wish is already decided")
	// ErrWishTxNotFound is a wish not bought in the transaction.
	ErrWishTxNotFound = errors.New("the wish isn't the transaction's")
)

// LinkWishesToTx marks the waiting wishes bought in the transaction, all or
// none (ErrWishNotWaiting).
func (r *Repository) LinkWishesToTx(ctx context.Context, txID string, wishIDs []string) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		for _, id := range wishIDs {
			n, err := qtx.LinkWishlistItemToTx(ctx, queries.LinkWishlistItemToTxParams{
				ID:            id,
				TransactionID: sql.NullString{String: txID, Valid: true},
			})
			if err != nil {
				logger.FromCtx(ctx).Error("linking a wish to a transaction", zap.Error(err), zap.String("id", id))
				return err
			}
			if n == 0 {
				return ErrWishNotWaiting
			}
		}
		return nil
	})
}

// UnlinkWishFromTx puts a wish bought in the transaction back to waiting.
func (r *Repository) UnlinkWishFromTx(ctx context.Context, txID, wishID string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	n, err := q.UnlinkWishlistItemFromTx(ctx, queries.UnlinkWishlistItemFromTxParams{
		ID:            wishID,
		TransactionID: sql.NullString{String: txID, Valid: true},
	})
	if err != nil {
		logger.FromCtx(ctx).Error("unlinking a wish", zap.Error(err), zap.String("id", wishID))
		return err
	}
	if n == 0 {
		return ErrWishTxNotFound
	}
	return nil
}

// GetWishTxCandidates are the expenses a wish may have been bought in,
// newest first, matching q when set.
func (r *Repository) GetWishTxCandidates(ctx context.Context, q string, limit int) ([]models.Transaction, error) {
	f := models.TxFilter{Q: q, Kind: models.ExpenseTxKind}
	p := txFilterParams(&f)
	p.Lim = int64(limit * 2)
	txs, err := r.listTransactions(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]models.Transaction, 0, limit)
	for _, t := range txs {
		if t.CanHoldWishes() && len(out) < limit {
			out = append(out, t)
		}
	}
	_, dbq, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if err := loadTxItemCounts(ctx, dbq, out); err != nil {
		return nil, err
	}
	if err := loadTxWishes(ctx, dbq, out); err != nil {
		return nil, err
	}
	return out, nil
}

// loadTxWishes sets the wishes bought in each transaction.
func loadTxWishes(ctx context.Context, q *queries.Queries, lists ...[]models.Transaction) error {
	var ids []string
	type ref struct{ list, i int }
	at := map[string]ref{}
	for li, txs := range lists {
		for i := range txs {
			ids = append(ids, txs[i].ID)
			at[txs[i].ID] = ref{li, i}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.GetWishlistItemsOfTxs(ctx, nullStrings(ids))
	if err != nil {
		logger.FromCtx(ctx).Error("querying the transactions' wishes", zap.Error(err))
		return err
	}
	for _, row := range rows {
		if r, ok := at[row.WishlistItem.TransactionID.String]; ok {
			tx := &lists[r.list][r.i]
			tx.Wishes = append(tx.Wishes, *dbWishlistItemToWishlistItem(&row.WishlistItem, &row.Currency))
		}
	}
	return nil
}
