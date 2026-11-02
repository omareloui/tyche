package repository

import (
	"context"
	"slices"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

const dayLayout = "2006-01-02"

// GetFlow is the fulfilled income and spending of the last
// models.FlowMonths months, in def.
func (r *Repository) GetFlow(ctx context.Context, def *models.Currency) (*models.Flow, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	today := time.Now()
	rows, err := q.GetDailyFlow(ctx, models.FlowFrom(today))
	if err != nil {
		l.Error("querying daily flow", zap.Error(err))
		return nil, err
	}

	var days []models.DayFlow
	var partial bool
	for _, row := range rows {
		day, err := time.Parse(dayLayout, row.Day)
		if err != nil {
			continue
		}
		cur := dbCurrencyToCurrency(&row.Currency, false)
		in, okIn := models.Convert(row.Income, cur, def.String)
		out, okOut := models.Convert(row.Expenses, cur, def.String)
		if !okIn || !okOut {
			partial = true
			continue
		}
		// Rows are sorted by day, a day's currencies come one after another.
		if n := len(days); n > 0 && days[n-1].Day.Equal(day) {
			days[n-1].In += in
			days[n-1].Out += out
			continue
		}
		days = append(days, models.DayFlow{Day: day, In: in, Out: out})
	}

	return models.NewFlow(today, days, partial), nil
}

// getCategoryTotals is the spending per category and currency of month.
func (r *Repository) getCategoryTotals(ctx context.Context, q *queries.Queries, month time.Time) ([]models.CategoryTotal, error) {
	rows, err := q.GetCategorySpending(ctx, queries.GetCategorySpendingParams{
		FromDate: month,
		ToDate:   month.AddDate(0, 1, 0),
	})
	if err != nil {
		logger.FromCtx(ctx).Error("querying category spending", zap.Error(err))
		return nil, err
	}
	totals := make([]models.CategoryTotal, len(rows))
	for i, row := range rows {
		totals[i] = models.CategoryTotal{
			CategoryID: row.CategoryID,
			Currency:   *dbCurrencyToCurrency(&row.Currency, false),
			Amount:     row.Total,
		}
	}
	return totals, nil
}

// GetCategorySpending is the spending per category of month, in def.
func (r *Repository) GetCategorySpending(ctx context.Context, month time.Time, def *models.Currency) (*models.CategorySpending, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	month = models.MonthOf(month)
	totals, err := r.getCategoryTotals(ctx, q, month)
	if err != nil {
		return nil, err
	}
	cats, err := r.GetCategories(ctx)
	if err != nil {
		return nil, err
	}

	cs := models.NewCategorySpending(month, *def, cats, totals)

	first, err := q.GetFirstFulfilledTransactionDate(ctx)
	if err != nil {
		l.Error("querying first transaction date", zap.Error(err))
		return nil, err
	}
	if d, err := time.Parse(dayLayout, first); err == nil && models.MonthOf(d).Before(month) {
		cs.Prev = month.AddDate(0, -1, 0).Format(monthLayout)
	}
	if next := month.AddDate(0, 1, 0); !next.After(time.Now()) {
		cs.Next = next.Format(monthLayout)
	}
	return cs, nil
}

// GetBudgetSummary is month's spending of the expense categories against
// their budgets, in def, hidden ones (category IDs or
// models.UncategorizedKey) left out of the totals.
func (r *Repository) GetBudgetSummary(ctx context.Context, month time.Time, def *models.Currency, hidden []string) (*models.BudgetSummary, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	month = models.MonthOf(month)
	rows, err := q.GetDailyCategorySpending(ctx, queries.GetDailyCategorySpendingParams{
		FromDate: month,
		ToDate:   month.AddDate(0, 1, 0),
	})
	if err != nil {
		l.Error("querying daily category spending", zap.Error(err))
		return nil, err
	}
	totals := make([]models.DayCategoryTotal, 0, len(rows))
	for _, row := range rows {
		day, err := time.Parse(dayLayout, row.Day)
		if err != nil {
			continue
		}
		totals = append(totals, models.DayCategoryTotal{
			Day: day,
			CategoryTotal: models.CategoryTotal{
				CategoryID: row.CategoryID,
				Currency:   *dbCurrencyToCurrency(&row.Currency, false),
				Amount:     row.Total,
			},
		})
	}
	cats, err := r.GetCategories(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	bs := models.NewBudgetSummary(month, now, *def, cats, totals, hidden)

	first, err := q.GetFirstFulfilledTransactionDate(ctx)
	if err != nil {
		l.Error("querying first transaction date", zap.Error(err))
		return nil, err
	}
	if d, err := time.Parse(dayLayout, first); err == nil && models.MonthOf(d).Before(month) {
		bs.Prev = month.AddDate(0, -1, 0).Format(monthLayout)
	}
	if next := month.AddDate(0, 1, 0); !next.After(now) {
		bs.Next = next.Format(monthLayout)
	}
	return bs, nil
}

// GetBudgets is the current month's usage of the budgeted categories.
func (r *Repository) GetBudgets(ctx context.Context, def *models.Currency) (*models.Budgets, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	today := time.Now()
	totals, err := r.getCategoryTotals(ctx, q, models.MonthOf(today))
	if err != nil {
		return nil, err
	}
	cats, err := r.GetCategories(ctx)
	if err != nil {
		return nil, err
	}
	return models.NewBudgets(today, *def, cats, totals), nil
}

// GetRecentTransactions is the n latest fulfilled transactions.
func (r *Repository) GetRecentTransactions(ctx context.Context, n int) ([]models.Transaction, error) {
	return r.listTransactions(ctx, queries.GetTransactionsParams{Fulfilled: true, Lim: int64(n)})
}

// GetUpcomingTransactions is the scheduled transactions due before until,
// overdue ones included, soonest first.
func (r *Repository) GetUpcomingTransactions(ctx context.Context, until time.Time) ([]models.Transaction, error) {
	txs, err := r.listTransactions(ctx, queries.GetTransactionsParams{
		Fulfilled: false,
		Before:    until.UTC(),
		Lim:       -1,
	})
	if err != nil {
		return nil, err
	}
	slices.Reverse(txs)
	return txs, nil
}

func (r *Repository) listTransactions(ctx context.Context, p queries.GetTransactionsParams) ([]models.Transaction, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.GetTransactions(ctx, p)
	if err != nil {
		logger.FromCtx(ctx).Error("querying transactions", zap.Error(err))
		return nil, err
	}
	txs := make([]models.Transaction, len(rows))
	for i := range rows {
		txs[i] = *dbTransactionRowToTransaction(&rows[i])
	}
	return txs, nil
}

// CountTransactionsNeedingReview counts the transactions, from sms mostly,
// waiting to be reviewed.
func (r *Repository) CountTransactionsNeedingReview(ctx context.Context) (int, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountTransactionsNeedingReview(ctx)
	if err != nil {
		logger.FromCtx(ctx).Error("counting transactions to review", zap.Error(err))
		return 0, err
	}
	return int(n), nil
}

// CountSMSNeedingAttention counts the sms that created no transaction and
// weren't dismissed.
func (r *Repository) CountSMSNeedingAttention(ctx context.Context) (int, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountSMSMessagesByStatus(ctx, []string{
		string(models.SMSStatusUnparsed),
		string(models.SMSStatusUnmatched),
		string(models.SMSStatusFailed),
	})
	if err != nil {
		logger.FromCtx(ctx).Error("counting sms needing attention", zap.Error(err))
		return 0, err
	}
	return int(n), nil
}
