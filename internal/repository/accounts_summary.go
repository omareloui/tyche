package repository

import (
	"context"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// GetAccountsSummary is the net worth of the accounts in the balance (and
// the ones with asks for), what they hold per kind and currency, its last
// months forecast to the end of next month, and the cards' and debts'
// state, in def.
func (r *Repository) GetAccountsSummary(ctx context.Context, def *models.Currency, with models.AccountsSummaryWith) (*models.AccountsSummary, error) {
	l := logger.FromCtx(ctx)
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	accs, err := r.GetAccounts(ctx)
	if err != nil {
		return nil, err
	}
	if err := r.AttachCardStatements(ctx, accs); err != nil {
		return nil, err
	}
	// Planned deposits come out of the live accounts only.
	live := accs
	// The goals' accounts, hidden from GetAccounts.
	goals, err := r.GetGoals(ctx)
	if err != nil {
		return nil, err
	}
	for _, gl := range goals {
		if gl.Account != nil {
			accs = append(accs, *gl.Account)
		}
	}
	loc, err := r.GetTimezone(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	today := now.In(loc)
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	start := monthStartFromCtx(ctx)
	cur := start.Of(today)
	next := cur.Next()

	rows, err := q.GetAccountsDailyNet(ctx, models.AccountHistoryFrom(start, today))
	if err != nil {
		l.Error("querying accounts daily net", zap.Error(err))
		return nil, err
	}
	daily := map[string][]models.DayFlow{}
	for _, row := range rows {
		if d, ok := dayFlow(row.Day, row.MoneyIn, row.MoneyOut); ok {
			daily[row.AccountID] = append(daily[row.AccountID], d)
		}
	}

	// The forecast, to the end of next month: the scheduled transactions,
	// the recurrent ones' occurrences not scheduled yet and the goal plans'
	// deposits, as if scheduled (kept apart to tell them in its info).
	scheduledRows, err := q.GetAccountsScheduledDailyNet(ctx, next.To)
	if err != nil {
		l.Error("querying accounts scheduled daily net", zap.Error(err))
		return nil, err
	}
	scheduled := map[string][]models.DayFlow{}
	for _, row := range scheduledRows {
		if d, ok := dayFlow(row.Day, row.MoneyIn, row.MoneyOut); ok {
			scheduled[row.AccountID] = append(scheduled[row.AccountID], d)
		}
	}

	rts, err := r.GetRecurrentTxs(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*models.Account, len(accs))
	for i := range accs {
		byID[accs[i].ID] = &accs[i]
	}
	recurrent := map[string][]models.DayFlow{}
	recurrentCount := 0
	// Their expenses also take from their budgets, their incomes maybe are
	// the salary.
	planned := []models.DayCategoryTotal{}
	incomes := []models.Balance{}
	for _, rt := range rts {
		a := byID[rt.Account.ID]
		if a == nil {
			continue
		}
		for _, d := range rt.OccurrencesBetween(todayDate, next.To) {
			recurrentCount++
			flow := models.DayFlow{Day: d}
			if rt.Kind == models.IncomeTxKind || rt.Kind == models.RefundTxKind || rt.Kind == models.AdjustmentTxKind {
				flow.In = rt.Amount
			} else {
				flow.Out = rt.Amount
			}
			recurrent[a.ID] = append(recurrent[a.ID], flow)
			if (rt.Kind == models.TransferTxKind || rt.Kind == models.DebtTxKind) && rt.ToAccount != nil {
				amount := rt.ToAmount
				if amount == 0 {
					amount = rt.Amount
				}
				recurrent[rt.ToAccount.ID] = append(recurrent[rt.ToAccount.ID], models.DayFlow{Day: d, In: amount})
			}
			switch {
			case rt.Kind == models.ExpenseTxKind && rt.Category != nil:
				planned = append(planned, models.DayCategoryTotal{Day: d, CategoryTotal: models.CategoryTotal{
					CategoryID: rt.Category.ID, Currency: a.Currency, Amount: rt.Amount,
				}})
			case rt.Kind == models.IncomeTxKind && next.Contains(d):
				incomes = append(incomes, models.Balance{Amount: rt.Amount, Currency: a.Currency})
			}
		}
	}

	deposits := models.PlannedDeposits(goals, live, start, now, next.To)
	goalFlows := map[string][]models.DayFlow{}
	for _, d := range deposits {
		goalFlows[d.Source.ID] = append(goalFlows[d.Source.ID], models.DayFlow{Day: d.On, Out: d.SourceTotal})
		goalFlows[d.Goal.Account.ID] = append(goalFlows[d.Goal.Account.ID], models.DayFlow{Day: d.On, In: d.Amount})
	}

	all := map[string][]models.DayFlow{}
	for _, m := range []map[string][]models.DayFlow{scheduled, recurrent, goalFlows} {
		for id, days := range m {
			all[id] = append(all[id], days...)
		}
	}
	s := models.NewAccountsSummary(accs, *def, with, start, today, next.Last(), daily, all)
	s.Info.Scheduled = s.NetFlow(scheduled)
	s.Info.Recurrent, s.Info.RecurrentCount = s.NetFlow(recurrent), recurrentCount
	s.Info.GoalDeposits, s.Info.GoalDepositCount = s.NetFlow(goalFlows), len(deposits)

	bs, err := r.GetBudgetSummary(ctx, cur.Month, def, nil)
	if err != nil {
		return nil, err
	}
	s.BudgetLeft = bs.Unspent()
	nextBs, err := r.GetBudgetSummary(ctx, next.Month, def, nil)
	if err != nil {
		return nil, err
	}
	plannedRows, err := q.GetScheduledCategorySpending(ctx, queries.GetScheduledCategorySpendingParams{
		FromDate: today.Format(time.DateOnly),
		ToDate:   next.To,
	})
	if err != nil {
		l.Error("querying scheduled category spending", zap.Error(err))
		return nil, err
	}
	for _, row := range plannedRows {
		day, err := time.Parse(dayLayout, row.Day)
		if err != nil {
			continue
		}
		planned = append(planned, models.DayCategoryTotal{Day: day, CategoryTotal: models.CategoryTotal{
			CategoryID: row.CategoryID, Currency: *dbCurrencyToCurrency(&row.Currency, false), Amount: row.Total,
		}})
	}
	s.Forecast(bs, nextBs, planned)

	// The salary, if set, comes on the first day of next month.
	ws, err := r.GetWorkSettings(ctx)
	if err != nil {
		return nil, err
	}
	if ws.MonthlySalary > 0 {
		salary, ok := models.Convert(ws.MonthlySalary, &ws.Currency, def.String)
		if !ok {
			s.Partial = true
			return s, nil
		}
		incomeRows, err := q.GetScheduledIncomes(ctx, queries.GetScheduledIncomesParams{FromDate: next.From, ToDate: next.To})
		if err != nil {
			l.Error("querying scheduled incomes", zap.Error(err))
			return nil, err
		}
		for _, row := range incomeRows {
			incomes = append(incomes, models.Balance{Amount: row.Amount, Currency: *dbCurrencyToCurrency(&row.Currency, false)})
		}
		s.ForecastSalary(salary, next.From, incomes)
	}
	return s, nil
}
