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
	start := monthStartFromCtx(ctx)
	first := start.Plus(today, -models.VisaMonths)
	days, partial, err := dailyFlowSince(ctx, q, first.From, def)
	if err != nil {
		return nil, err
	}
	m := &models.VisaMoney{Months: make([]float64, models.VisaMonths), Periods: make([]models.Period, models.VisaMonths), Partial: partial}
	for i, p := 0, first; i < models.VisaMonths; i, p = i+1, p.Next() {
		m.Periods[i] = p
	}
	for _, d := range days {
		for i, p := range m.Periods {
			if !d.Day.Before(p.From) && d.Day.Before(p.To) {
				m.Months[i] += d.In
				break
			}
		}
	}

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
