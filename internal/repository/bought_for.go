package repository

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/omareloui/tyche/internal/formatters"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// CreateBoughtForMe saves something someone bought for the user: the expense
// and the debt borrowed from them for it, linked.
func (r *Repository) CreateBoughtForMe(ctx context.Context, expense, debt *models.Transaction) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		if err := r.createTransactionAndApplyToAccountIfNeededWithTxn(ctx, qtx, expense); err != nil {
			return err
		}
		if err := autoTagWithTxn(ctx, qtx, expense); err != nil {
			return err
		}
		if err := r.createTransactionAndApplyToAccountIfNeededWithTxn(ctx, qtx, debt); err != nil {
			return err
		}
		if err := qtx.SetTransactionBoughtFor(ctx, queries.SetTransactionBoughtForParams{
			ID:          debt.ID,
			BoughtForID: sql.NullString{String: expense.ID, Valid: true},
		}); err != nil {
			logger.FromCtx(ctx).Error("linking bought for", zap.Error(err), zap.String("debt_id", debt.ID), zap.String("expense_id", expense.ID))
			return err
		}
		debt.BoughtForID = expense.ID
		return nil
	})
}

// LinkBoughtFor marks the debt debtID as borrowed for (part of) the expense
// expenseID: what someone bought for the user.
func (r *Repository) LinkBoughtFor(ctx context.Context, debtID, expenseID string) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)

		debt, err := r.getTransactionWithTxn(ctx, qtx, debtID)
		if err != nil {
			return err
		}
		expense, err := r.getTransactionWithTxn(ctx, qtx, expenseID)
		if err != nil {
			return err
		}
		others, err := qtx.GetBoughtForTotal(ctx, queries.GetBoughtForTotalParams{
			ID:        sql.NullString{String: expenseID, Valid: true},
			ExcludeID: debtID,
		})
		if err != nil {
			l.Error("querying bought for total", zap.Error(err), zap.String("id", expenseID))
			return err
		}
		if err := models.ValidateBoughtFor(debt, expense, others); err != nil {
			return err
		}

		if err := qtx.SetTransactionBoughtFor(ctx, queries.SetTransactionBoughtForParams{
			ID:          debtID,
			BoughtForID: sql.NullString{String: expenseID, Valid: true},
		}); err != nil {
			l.Error("linking bought for", zap.Error(err), zap.String("debt_id", debtID), zap.String("expense_id", expenseID))
			return err
		}
		l.Debug("linked bought for", zap.String("debt_id", debtID), zap.String("expense_id", expenseID))
		return nil
	})
}

// UnlinkBoughtFor makes the debt id a plain debt again.
func (r *Repository) UnlinkBoughtFor(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	if err := q.SetTransactionBoughtFor(ctx, queries.SetTransactionBoughtForParams{ID: id}); err != nil {
		l.Error("unlinking bought for", zap.Error(err), zap.String("id", id))
		return err
	}
	l.Debug("unlinked bought for", zap.String("id", id))
	return nil
}

// getTransactionWithTxn is the transaction id, as the list reads it.
func (r *Repository) getTransactionWithTxn(ctx context.Context, qtx *queries.Queries, id string) (*models.Transaction, error) {
	row, err := qtx.GetTransaction(ctx, id)
	if err != nil {
		logger.FromCtx(ctx).Error("querying transaction", zap.Error(err), zap.String("id", id))
		return nil, err
	}
	return dbTransactionRowToTransaction(&row), nil
}

// GetBoughtByCandidates returns the debts borrowed into the expense's
// currency around its day, not linked to an expense yet, that the expense
// could have been bought with.
func (r *Repository) GetBoughtByCandidates(ctx context.Context, expense *models.Transaction) ([]models.Transaction, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	day := startOfDay(expense.ScheduledTo)
	rows, err := q.GetBoughtByCandidates(ctx, queries.GetBoughtByCandidatesParams{
		CurrencyID: expense.Account.Currency.ID,
		Since:      day.Add(-models.BoughtForWindow).UTC(),
		Until:      day.AddDate(0, 0, 1).Add(models.BoughtForWindow).UTC(),
	})
	if err != nil {
		logger.FromCtx(ctx).Error("querying bought by candidates", zap.Error(err))
		return nil, err
	}
	txs := make([]models.Transaction, 0, len(rows))
	for i := range rows {
		txs = append(txs, *dbTransactionRowToTransaction(&rows[i]))
	}
	return txs, nil
}

// GetBoughtForCandidates returns the expenses around the debt's day, in the
// currency it brought in, with at least its amount not bought by someone yet.
func (r *Repository) GetBoughtForCandidates(ctx context.Context, debt *models.Transaction) ([]models.BoughtForCandidate, error) {
	if debt.ToAccount == nil {
		return nil, nil
	}
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	day := startOfDay(debt.ScheduledTo)
	rows, err := q.GetBoughtForCandidates(ctx, queries.GetBoughtForCandidatesParams{
		CurrencyID: debt.ToAccount.Currency.ID,
		Since:      day.Add(-models.BoughtForWindow).UTC(),
		Until:      day.AddDate(0, 0, 1).Add(models.BoughtForWindow).UTC(),
		MinLeft:    debt.ToAmount - amountEpsilon,
	})
	if err != nil {
		logger.FromCtx(ctx).Error("querying bought for candidates", zap.Error(err))
		return nil, err
	}

	exps := make([]models.BoughtForCandidate, 0, len(rows))
	for _, row := range rows {
		e := models.BoughtForCandidate{
			ID:            row.ID,
			Title:         row.Description.String,
			Category:      row.CategoryName.String,
			CategoryColor: row.CategoryColor.String,
			Account:       row.AccountName,
			Amount:        row.Amount,
			BoughtBy:      row.BoughtBy,
			Currency:      models.Currency{Symbol: row.CurrencySymbol, InGrams: row.CurrencyInGrams},
			ScheduledTo:   row.ScheduledTo,
		}
		if row.CounterpartyName.Valid {
			if e.Title == "" {
				e.Title = row.CounterpartyName.String
			} else {
				e.Subtitle = row.CounterpartyName.String
			}
		}
		if e.Title == "" {
			e.Title = models.ExpenseTxKind.String()
		}
		exps = append(exps, e)
	}
	return exps, nil
}

// loadBoughtBy nests the debts of the people who bought the expenses in lists
// under them, oldest first. They're left out of the list query, so an expense
// gets them even when they're in another chunk.
func (r *Repository) loadBoughtBy(ctx context.Context, q *queries.Queries, lists ...[]models.Transaction) error {
	var ids []string
	for _, txs := range lists {
		for _, t := range txs {
			if t.Kind == models.ExpenseTxKind {
				ids = append(ids, t.ID)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}

	b, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	rows, err := q.GetBoughtByOfMany(ctx, string(b))
	if err != nil {
		logger.FromCtx(ctx).Error("querying bought by debts", zap.Error(err))
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	byExpense := make(map[string][]models.Transaction)
	for i := range rows {
		d := dbTransactionRowToTransaction(&rows[i])
		byExpense[d.BoughtForID] = append(byExpense[d.BoughtForID], *d)
	}
	for _, txs := range lists {
		for i := range txs {
			txs[i].BoughtBy = byExpense[txs[i].ID]
		}
	}
	return nil
}

// syncBoughtForOnUpdateWithTxn keeps the bought for links of a transaction
// valid after updating it: an expense no longer one (or now a goal's) lets
// its debts go, and one below what its debts brought in is rejected; a debt
// no longer borrowed into the expense's currency is unlinked, and one over
// what's left of its expense is rejected.
func (r *Repository) syncBoughtForOnUpdateWithTxn(qtx *queries.Queries, ctx context.Context, oldTx, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)

	if oldTx.Kind != models.ExpenseTxKind && !oldTx.IsBoughtFor() {
		return nil
	}
	// The form's transaction only has its accounts' ids: read it back, as
	// saved.
	saved, err := r.getTransactionWithTxn(ctx, qtx, tx.ID)
	if err != nil {
		return err
	}

	if oldTx.Kind == models.ExpenseTxKind {
		total, err := qtx.GetBoughtForTotal(ctx, queries.GetBoughtForTotalParams{
			ID: sql.NullString{String: tx.ID, Valid: true},
		})
		if err != nil {
			l.Error("querying bought for total", zap.Error(err), zap.String("id", tx.ID))
			return err
		}
		if total > 0 {
			if !saved.CanBeBoughtBy() || saved.Account.Currency.ID != oldTx.Account.Currency.ID {
				if err := qtx.ClearBoughtForOf(ctx, sql.NullString{String: tx.ID, Valid: true}); err != nil {
					l.Error("clearing bought for debts", zap.Error(err), zap.String("id", tx.ID))
					return err
				}
				l.Debug("cleared bought for debts", zap.String("id", tx.ID))
			} else if total > saved.Amount+amountEpsilon {
				return reimbursedAmountError("lt_bought_by", formatters.FormatAmount(total, oldTx.Account.Currency.Symbol))
			}
		}
	}

	if !oldTx.IsBoughtFor() {
		return nil
	}

	expense, err := r.getTransactionWithTxn(ctx, qtx, oldTx.BoughtForID)
	if err != nil {
		return err
	}
	others, err := qtx.GetBoughtForTotal(ctx, queries.GetBoughtForTotalParams{
		ID:        sql.NullString{String: oldTx.BoughtForID, Valid: true},
		ExcludeID: tx.ID,
	})
	if err != nil {
		l.Error("querying bought for total", zap.Error(err), zap.String("id", oldTx.BoughtForID))
		return err
	}
	switch err := models.ValidateBoughtFor(saved, expense, others); err {
	case nil:
		return nil
	case models.ErrBoughtForExceeds:
		return reimbursedAmountError("gt_bought_for", formatters.FormatAmount(expense.Amount-others, expense.Account.Currency.Symbol))
	default:
		if err := qtx.SetTransactionBoughtFor(ctx, queries.SetTransactionBoughtForParams{ID: tx.ID}); err != nil {
			l.Error("unlinking bought for", zap.Error(err), zap.String("id", tx.ID))
			return err
		}
		l.Debug("unlinked bought for", zap.String("id", tx.ID))
		return nil
	}
}
