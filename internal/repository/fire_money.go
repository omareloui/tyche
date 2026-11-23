package repository

import (
	"context"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
)

// lastMonthsFlow is the income and spending of each of the last n full user
// months before today's, oldest first, in def (partial when a currency had
// no rate).
func lastMonthsFlow(ctx context.Context, q *queries.Queries, n int, def *models.Currency, today time.Time) (in, out []float64, periods []models.Period, partial bool, err error) {
	first := monthStartFromCtx(ctx).Plus(today, -n)
	days, partial, err := dailyFlowSince(ctx, q, first.From, def)
	if err != nil {
		return nil, nil, nil, false, err
	}
	in, out, periods = make([]float64, n), make([]float64, n), make([]models.Period, n)
	for i, p := 0, first; i < n; i, p = i+1, p.Next() {
		periods[i] = p
	}
	for _, d := range days {
		for i, p := range periods {
			if !d.Day.Before(p.From) && d.Day.Before(p.To) {
				in[i] += d.In
				out[i] += d.Out
				break
			}
		}
	}
	return in, out, periods, partial, nil
}

// GetFireMoney is what the FIRE calculator starts from, in def: the
// portfolio (the live accounts in the balance, with the excluded ones' and
// the goals' when with asks) and the last full months' income and spending.
func (r *Repository) GetFireMoney(ctx context.Context, def *models.Currency, today time.Time, with models.AccountsSummaryWith) (*models.FireMoney, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	in, out, periods, partial, err := lastMonthsFlow(ctx, q, models.FireMonths, def, today)
	if err != nil {
		return nil, err
	}
	m := &models.FireMoney{Income: in, Spending: out, Periods: periods, Partial: partial}

	accs, err := r.GetAccounts(ctx)
	if err != nil {
		return nil, err
	}
	goals, err := r.GetGoals(ctx)
	if err != nil {
		return nil, err
	}
	for i := range goals {
		gl := &goals[i]
		if gl.Account != nil {
			accs = append(accs, *gl.Account)
		}
		accs = append(accs, gl.Holdings...)
	}
	for i := range accs {
		a := &accs[i]
		if a.IsArchive || a.IsDeleted() {
			continue
		}
		if a.Kind.Enum == models.AccKindGoal {
			if m.Goals++; !with.Goals {
				continue
			}
		} else if a.ExcludeFromBalance {
			if m.Excluded++; !with.Excluded {
				continue
			}
		}
		v, ok := models.Convert(a.Amount, &a.Currency, def.String)
		if !ok {
			m.Partial = true
			continue
		}
		m.Portfolio += v
	}
	return m, nil
}
