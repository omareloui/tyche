package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/formatters"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// reimbursableSince limits how far back, from the income's day, expenses
// are offered to be reimbursed.
const reimbursableSince = 90 * 24 * time.Hour

// amountEpsilon absorbs float rounding when comparing amounts rounded to 2
// decimals.
const amountEpsilon = 0.005

// LinkReimbursement marks the income incomeID as paying back (part of) the
// expense expenseID.
func (r *Repository) LinkReimbursement(ctx context.Context, incomeID, expenseID string) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)

		income, err := qtx.GetTransaction(ctx, incomeID)
		if err != nil {
			l.Error("querying income to reimburse with", zap.Error(err), zap.String("id", incomeID))
			return err
		}
		expense, err := qtx.GetTransaction(ctx, expenseID)
		if err != nil {
			l.Error("querying expense to reimburse", zap.Error(err), zap.String("id", expenseID))
			return err
		}

		if models.TxKind(income.Transaction.Kind) != models.IncomeTxKind ||
			models.TxKind(expense.Transaction.Kind) != models.ExpenseTxKind {
			return models.ErrReimbursementKinds
		}
		if income.Transaction.TransferPairID.Valid || expense.Transaction.TransferPairID.Valid {
			return models.ErrReimbursementTransferPair
		}
		if expense.Transaction.GoalID.Valid {
			return models.ErrReimbursementGoal
		}
		if income.Account.CurrencyID != expense.Account.CurrencyID {
			return models.ErrReimbursementCurrency
		}

		reimbursed, err := qtx.GetReimbursedTotal(ctx, queries.GetReimbursedTotalParams{
			ID:        sql.NullString{String: expenseID, Valid: true},
			ExcludeID: incomeID,
		})
		if err != nil {
			l.Error("querying reimbursed total", zap.Error(err), zap.String("id", expenseID))
			return err
		}
		if reimbursed+income.Transaction.Amount > expense.Transaction.Amount+amountEpsilon {
			return models.ErrReimbursementExceeds
		}

		if err := qtx.SetTransactionReimburses(ctx, queries.SetTransactionReimbursesParams{
			ID:           incomeID,
			ReimbursesID: sql.NullString{String: expenseID, Valid: true},
		}); err != nil {
			l.Error("linking reimbursement", zap.Error(err), zap.String("income_id", incomeID), zap.String("expense_id", expenseID))
			return err
		}
		l.Debug("linked reimbursement", zap.String("income_id", incomeID), zap.String("expense_id", expenseID))
		return nil
	})
}

// UnlinkReimbursement makes the income id a plain income again.
func (r *Repository) UnlinkReimbursement(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	if err := q.SetTransactionReimburses(ctx, queries.SetTransactionReimbursesParams{ID: id}); err != nil {
		l.Error("unlinking reimbursement", zap.Error(err), zap.String("id", id))
		return err
	}
	l.Debug("unlinked reimbursement", zap.String("id", id))
	return nil
}

// GetReimbursableExpenses returns the expenses the income can reimburse
// (same currency, not a goal's, with enough left to be reimbursed) from the
// income's day and the days before it.
func (r *Repository) GetReimbursableExpenses(ctx context.Context, income *models.Transaction) ([]models.ReimbursableExpense, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dayEnd := startOfDay(income.ScheduledTo).AddDate(0, 0, 1)
	rows, err := q.GetReimbursableExpenses(ctx, queries.GetReimbursableExpensesParams{
		CurrencyID: income.Account.Currency.ID,
		Since:      dayEnd.Add(-reimbursableSince).UTC(),
		Until:      dayEnd.UTC(),
		MinLeft:    income.Amount - amountEpsilon,
	})
	if err != nil {
		l.Error("querying reimbursable expenses", zap.Error(err))
		return nil, err
	}

	exps := []models.ReimbursableExpense{}
	for _, row := range rows {
		e := models.ReimbursableExpense{
			ID:             row.ID,
			Title:          row.Description.String,
			Category:       row.CategoryName.String,
			CategoryColor:  row.CategoryColor.String,
			Account:        row.AccountName,
			Amount:         row.Amount,
			Reimbursed:     row.Reimbursed,
			CurrencySymbol: row.CurrencySymbol,
			ScheduledTo:    row.ScheduledTo,
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

// startOfDay returns the midnight starting t's day, in t's location.
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// syncReimbursementsOnUpdateWithTxn keeps the reimbursement links of a
// transaction valid after updating it: links that don't apply to its new kind
// (or goal) are dropped, and amounts that would leave an expense
// over-reimbursed are rejected.
func (r *Repository) syncReimbursementsOnUpdateWithTxn(qtx *queries.Queries, ctx context.Context, oldTx, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)

	if oldTx.Reimbursed > 0 {
		if tx.Kind != models.ExpenseTxKind || tx.Goal != nil {
			if err := qtx.ClearReimbursementsOf(ctx, sql.NullString{String: tx.ID, Valid: true}); err != nil {
				l.Error("clearing reimbursements", zap.Error(err), zap.String("id", tx.ID))
				return err
			}
			l.Debug("cleared reimbursements", zap.String("id", tx.ID))
		} else if oldTx.Reimbursed > tx.Amount+amountEpsilon {
			return reimbursedAmountError("lt_reimbursed", formatters.FormatAmount(oldTx.Reimbursed, oldTx.Account.Currency.Symbol))
		}
	}

	if !oldTx.IsReimbursement() {
		return nil
	}

	if tx.Kind != models.IncomeTxKind {
		if err := qtx.SetTransactionReimburses(ctx, queries.SetTransactionReimbursesParams{ID: tx.ID}); err != nil {
			l.Error("unlinking reimbursement", zap.Error(err), zap.String("id", tx.ID))
			return err
		}
		l.Debug("unlinked reimbursement", zap.String("id", tx.ID))
		return nil
	}

	expense, err := qtx.GetTransaction(ctx, oldTx.ReimbursesID)
	if err != nil {
		l.Error("querying reimbursed expense", zap.Error(err), zap.String("id", oldTx.ReimbursesID))
		return err
	}
	reimbursed, err := qtx.GetReimbursedTotal(ctx, queries.GetReimbursedTotalParams{
		ID:        sql.NullString{String: oldTx.ReimbursesID, Valid: true},
		ExcludeID: tx.ID,
	})
	if err != nil {
		l.Error("querying reimbursed total", zap.Error(err), zap.String("id", oldTx.ReimbursesID))
		return err
	}
	if left := expense.Transaction.Amount - reimbursed; tx.Amount > left+amountEpsilon {
		return reimbursedAmountError("gt_reimbursable", formatters.FormatAmount(left, expense.Currency.Symbol))
	}
	return nil
}

func reimbursedAmountError(tag, param string) *errs.ValidationError {
	return &errs.ValidationError{Errors: errs.ValidationErrors{"Amount": {Tag: tag, Param: param}}}
}

func dbReimbursementRowToTransaction(row *queries.GetReimbursementsOfRow) models.Transaction {
	var cat *queries.Category
	if row.Transaction.CategoryID.Valid {
		cat = &queries.Category{ID: row.Transaction.CategoryID.String, Name: row.CategoryName.String}
	}
	var cp *queries.Counterparty
	if row.Transaction.CounterpartyID.Valid {
		cp = &queries.Counterparty{
			ID:    row.Transaction.CounterpartyID.String,
			Name:  row.CounterpartyName.String,
			Icon:  row.CounterpartyIcon,
			Color: row.CounterpartyColor,
		}
	}
	return *dbTransactionToTransaction(&row.Transaction, &row.Account, &row.Currency, nil, nil, cat, cp, nil, nil)
}
