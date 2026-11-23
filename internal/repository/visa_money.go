package repository

import (
	"context"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

// GetVisaMoney is the user's income of the last full months and savings,
// in def.
func (r *Repository) GetVisaMoney(ctx context.Context, def *models.Currency, today time.Time) (*models.VisaMoney, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	in, _, periods, partial, err := lastMonthsFlow(ctx, q, models.VisaMonths, def, today)
	if err != nil {
		return nil, err
	}
	m := &models.VisaMoney{Months: in, Periods: periods, Partial: partial}

	bal, err := r.GetBalances(ctx)
	if err != nil {
		return nil, err
	}
	for _, b := range bal {
		v, ok := models.Convert(b.Amount, &b.Currency, def.String)
		if !ok {
			m.Partial = true
			continue
		}
		m.Savings += v
	}
	goals, err := r.GetGoals(ctx)
	if err != nil {
		return nil, err
	}
	for i := range goals {
		gl := &goals[i]
		if !gl.FulfilledAt.IsZero() {
			continue
		}
		v, ok := models.Convert(gl.Balance(), &gl.Currency, def.String)
		if !ok {
			m.Partial = true
			continue
		}
		m.Savings += v
	}
	return m, nil
}
