package repository

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// GetMonthRecap sums up the user's month p against the one before and the
// average of the models.RecapAvgMonths before it, in def at today's rates;
// today is the user's.
func (r *Repository) GetMonthRecap(ctx context.Context, p models.Period, today time.Time, def *models.Currency) (*models.MonthRecap, error) {
	l := logger.FromCtx(ctx)
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	rc := &models.MonthRecap{Period: p, Today: today, Currency: *def}

	firstDay, err := q.GetFirstFulfilledTransactionDate(ctx)
	if err != nil {
		l.Error("querying first transaction date", zap.Error(err))
		return nil, err
	}
	first, _ := time.Parse(dayLayout, firstDay)
	if !first.IsZero() && first.Before(p.From) {
		rc.PrevMonth = p.Prev().Key()
	}
	if n := p.Next(); !n.From.After(today) {
		rc.NextMonth = n.Key()
	}

	avg := models.RecapAvgPeriods(p, first)
	rc.AvgMonths = len(avg)
	from := p.Prev().From
	if len(avg) > 0 {
		from = avg[len(avg)-1].From
	}

	// Income and spending.
	days, partial, err := dailyFlowSince(ctx, q, from, def)
	if err != nil {
		return nil, err
	}
	rc.Partial = rc.Partial || partial
	rc.Flow, rc.Prev, rc.Avg = models.NewRecapFlow(p, avg, days)
	last := p.Last()
	if rc.IsOpen() {
		last = today
	}
	rc.NoSpendDays, rc.Busiest = models.RecapDays(p, last, days)

	// Categories against the average, and the budgets.
	catRows, err := q.GetDailyCategorySpending(ctx, queries.GetDailyCategorySpendingParams{FromDate: from, ToDate: p.To})
	if err != nil {
		l.Error("querying daily category spending", zap.Error(err))
		return nil, err
	}
	totals := make([]models.DayCategoryTotal, 0, len(catRows))
	var monthTotals []models.DayCategoryTotal
	for _, row := range catRows {
		day, err := time.Parse(dayLayout, row.Day)
		if err != nil {
			continue
		}
		t := models.DayCategoryTotal{Day: day, CategoryTotal: models.CategoryTotal{
			CategoryID: row.CategoryID,
			Currency:   *dbCurrencyToCurrency(&row.Currency, false),
			Amount:     row.Total,
		}}
		totals = append(totals, t)
		if p.Contains(day) {
			monthTotals = append(monthTotals, t)
		}
	}
	cats, err := r.GetCategories(ctx)
	if err != nil {
		return nil, err
	}
	rc.CategoryChanges, partial = models.NewRecapCategoryChanges(p, avg, *def, cats, totals)
	rc.Partial = rc.Partial || partial
	rc.Budgets = models.NewBudgetSummary(p, today, *def, cats, monthTotals, nil)
	rc.Partial = rc.Partial || rc.Budgets.Partial

	// New merchants.
	if rc.NewMerchants, err = r.recapNewMerchants(ctx, q, p, def); err != nil {
		return nil, err
	}

	// Largest expenses.
	expRows, err := q.GetRecapExpenses(ctx, queries.GetRecapExpensesParams{FromDate: p.From, ToDate: p.To})
	if err != nil {
		l.Error("querying the recap's expenses", zap.Error(err))
		return nil, err
	}
	txs := make([]models.Transaction, len(expRows))
	for i, row := range expRows {
		txs[i] = recapExpense(row)
	}
	rc.Largest, partial = models.LargestExpenses(txs, *def, models.RecapTop)
	rc.Partial = rc.Partial || partial

	// Goals.
	goals, err := r.GetGoals(ctx)
	if err != nil {
		return nil, err
	}
	goalRows, err := q.GetGoalContributionsBetween(ctx, queries.GetGoalContributionsBetweenParams{FromDate: p.From, ToDate: p.To})
	if err != nil {
		l.Error("querying goal contributions", zap.Error(err))
		return nil, err
	}
	byID := map[string]*models.Goal{}
	for i := range goals {
		byID[goals[i].ID] = &goals[i]
	}
	deposited := map[string]float64{}
	for _, row := range goalRows {
		if gl := byID[row.GoalID.String]; gl != nil {
			deposited[gl.ID] += gl.ValueOf(row.Amount, &models.Currency{String: row.Currency})
		}
	}
	rc.Goals = models.NewRecapGoals(goals, deposited, p)

	// Net worth, worked back from today's balances.
	accs, err := r.GetAccounts(ctx)
	if err != nil {
		return nil, err
	}
	netRows, err := q.GetAccountsDailyNet(ctx, p.From)
	if err != nil {
		l.Error("querying accounts daily net", zap.Error(err))
		return nil, err
	}
	daily := map[string][]models.DayFlow{}
	for _, row := range netRows {
		if d, ok := dayFlow(row.Day, row.MoneyIn, row.MoneyOut); ok {
			daily[row.AccountID] = append(daily[row.AccountID], d)
		}
	}
	rc.NetStart, rc.NetEnd, partial = models.RecapNetWorth(accs, *def, daily, p)
	rc.Partial = rc.Partial || partial

	return rc, nil
}

// recapNewMerchants are the merchants first paid in p with what was spent at
// them, in def.
func (r *Repository) recapNewMerchants(ctx context.Context, q *queries.Queries, p models.Period, def *models.Currency) (*models.MerchantSpending, error) {
	rows, err := q.GetFirstSeenCounterparties(ctx, queries.GetFirstSeenCounterpartiesParams{FromDate: p.From, ToDate: p.To})
	if err != nil {
		logger.FromCtx(ctx).Error("querying first seen counterparties", zap.Error(err))
		return nil, err
	}
	ms := &models.MerchantSpending{Month: p.Month, Period: p, Currency: *def}
	idx := map[string]int{}
	for _, row := range rows {
		amount, ok := models.Convert(row.Total, dbCurrencyToCurrency(&row.Currency, false), def.String)
		if !ok {
			continue
		}
		if i, ok := idx[row.CounterpartyID]; ok {
			ms.Items[i].Amount += amount
			continue
		}
		idx[row.CounterpartyID] = len(ms.Items)
		ms.Items = append(ms.Items, models.MerchantSpend{
			Counterparty: models.Counterparty{
				ID:         row.CounterpartyID,
				Name:       row.CounterpartyName,
				Icon:       row.CounterpartyIcon.String,
				Color:      row.CounterpartyColor.String,
				IsMerchant: true,
			},
			Amount: amount,
		})
	}
	ms.Items = slices.DeleteFunc(ms.Items, func(it models.MerchantSpend) bool { return it.Amount <= 0 })
	slices.SortStableFunc(ms.Items, func(a, b models.MerchantSpend) int { return cmp.Compare(b.Amount, a.Amount) })
	return ms, nil
}

// recapExpense is the expense of a GetRecapExpenses row.
func recapExpense(row queries.GetRecapExpensesRow) models.Transaction {
	t := models.Transaction{
		ID:             row.ID,
		Kind:           models.ExpenseTxKind,
		Amount:         row.Amount,
		Reimbursed:     row.Reimbursed,
		Description:    row.Description.String,
		ScheduledTo:    row.ScheduledTo,
		TransferPairID: row.TransferPairID.String,
		Account: models.Account{
			ID:       row.AccountID,
			Name:     row.AccountName,
			Currency: *dbCurrencyToCurrency(&row.Currency, false),
		},
	}
	if row.CounterpartyID.Valid {
		t.Counterparty = &models.Counterparty{
			ID:    row.CounterpartyID.String,
			Name:  row.CounterpartyName.String,
			Icon:  row.CounterpartyIcon.String,
			Color: row.CounterpartyColor.String,
		}
	}
	if row.CategoryID.Valid {
		t.Category = &models.Category{ID: row.CategoryID.String, Name: row.CategoryName.String, Color: row.CategoryColor.String}
	}
	return t
}
