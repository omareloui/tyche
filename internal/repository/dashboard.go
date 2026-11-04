package repository

import (
	"context"
	"slices"
	"time"

	"github.com/omareloui/tyche/internal/badge"
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
	start := monthStartFromCtx(ctx)
	rows, err := q.GetDailyFlow(ctx, models.FlowFrom(start, today))
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

	return models.NewFlow(start, today, days, partial), nil
}

// getCategoryTotals is the spending per category and currency of p.
func (r *Repository) getCategoryTotals(ctx context.Context, q *queries.Queries, p models.Period) ([]models.CategoryTotal, error) {
	rows, err := q.GetCategorySpending(ctx, queries.GetCategorySpendingParams{
		FromDate: p.From,
		ToDate:   p.To,
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

// GetCategorySpending is the spending per category of the user's month named
// after month's (the current one when zero), in def.
func (r *Repository) GetCategorySpending(ctx context.Context, month time.Time, def *models.Currency) (*models.CategorySpending, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	p := userPeriod(ctx, month)
	totals, err := r.getCategoryTotals(ctx, q, p)
	if err != nil {
		return nil, err
	}
	cats, err := r.GetCategories(ctx)
	if err != nil {
		return nil, err
	}

	cs := models.NewCategorySpending(p, *def, cats, totals)
	if cs.Prev, cs.Next, err = r.periodNav(ctx, q, p, time.Now()); err != nil {
		return nil, err
	}
	return cs, nil
}

// GetBudgetSummary is the spending of the user's month named after month's
// (the current one when zero) of the expense categories against
// their budgets, in def, hidden ones (category IDs or
// models.UncategorizedKey) left out of the totals.
func (r *Repository) GetBudgetSummary(ctx context.Context, month time.Time, def *models.Currency, hidden []string) (*models.BudgetSummary, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	p := userPeriod(ctx, month)
	rows, err := q.GetDailyCategorySpending(ctx, queries.GetDailyCategorySpendingParams{
		FromDate: p.From,
		ToDate:   p.To,
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
	bs := models.NewBudgetSummary(p, now, *def, cats, totals, hidden)
	if bs.Prev, bs.Next, err = r.periodNav(ctx, q, p, now); err != nil {
		return nil, err
	}
	return bs, nil
}

// userPeriod is the user's month named after month's, the current one when
// month is zero.
func userPeriod(ctx context.Context, month time.Time) models.Period {
	s := monthStartFromCtx(ctx)
	if month.IsZero() {
		return s.Of(time.Now())
	}
	return s.For(month)
}

// periodNav is the months (YYYY-MM) before and after p to navigate to, if
// any: back to the first fulfilled transaction's, up to now's.
func (r *Repository) periodNav(ctx context.Context, q *queries.Queries, p models.Period, now time.Time) (prev, next string, err error) {
	first, err := q.GetFirstFulfilledTransactionDate(ctx)
	if err != nil {
		logger.FromCtx(ctx).Error("querying first transaction date", zap.Error(err))
		return "", "", err
	}
	if d, err := time.Parse(dayLayout, first); err == nil && d.Before(p.From) {
		prev = p.Prev().Key()
	}
	if n := p.Next(); !n.From.After(now) {
		next = n.Key()
	}
	return prev, next, nil
}

// GetBudgets is the current month's usage of the budgeted categories.
func (r *Repository) GetBudgets(ctx context.Context, def *models.Currency) (*models.Budgets, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	today := time.Now()
	start := monthStartFromCtx(ctx)
	totals, err := r.getCategoryTotals(ctx, q, start.Of(today))
	if err != nil {
		return nil, err
	}
	cats, err := r.GetCategories(ctx)
	if err != nil {
		return nil, err
	}
	return models.NewBudgets(start, today, *def, cats, totals), nil
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

// GetBadgeCounts is what the installed apps' badge counts: the sms needing
// attention, the transactions to review and the overdue scheduled ones (due
// before today, UTC like the transactions list).
func (r *Repository) GetBadgeCounts(ctx context.Context) (badge.Counts, error) {
	sms, err := r.CountSMSNeedingAttention(ctx)
	if err != nil {
		return badge.Counts{}, err
	}
	review, err := r.CountTransactionsNeedingReview(ctx)
	if err != nil {
		return badge.Counts{}, err
	}
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return badge.Counts{}, err
	}
	now := time.Now().UTC()
	overdue, err := q.CountOverdueTransactions(ctx, time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC))
	if err != nil {
		logger.FromCtx(ctx).Error("counting overdue transactions", zap.Error(err))
		return badge.Counts{}, err
	}
	return badge.Counts{SMS: sms, Review: review, Overdue: int(overdue)}, nil
}
