package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

func (r *Repository) GetWorkSettings(ctx context.Context) (*models.WorkSettings, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	row, err := q.GetWorkSettings(ctx)
	if err != nil {
		l.Error("fetching work settings", zap.Error(err))
		return nil, err
	}

	return &models.WorkSettings{
		MonthlySalary: row.MonthlySalary.Float64,
		Currency: models.Currency{
			ID:     row.CurrencyID.String,
			String: row.CurrencyString.String,
			Symbol: row.CurrencySymbol.String,
		},
		HoursPerWeek: row.WorkHoursPerWeek,
	}, nil
}

func (r *Repository) SetWorkSettings(ctx context.Context, ws *models.WorkSettings) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	return q.SetWorkSettings(ctx, queries.SetWorkSettingsParams{
		MonthlySalary:    sql.NullFloat64{Float64: ws.MonthlySalary, Valid: ws.MonthlySalary > 0},
		SalaryCurrencyID: sql.NullString{String: ws.Currency.ID, Valid: ws.Currency.ID != ""},
		WorkHoursPerWeek: ws.HoursPerWeek,
	})
}

// goalPaceMonths is how far back a goal's contributions count towards its
// monthly pace.
const goalPaceMonths = 3

// GetGoalPaces is each goal's average monthly contribution over the last
// few months, in the goal's currency.
func (r *Repository) GetGoalPaces(ctx context.Context, goals []models.Goal) (models.GoalPaces, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := q.GetGoalContributionsSince(ctx, time.Now().AddDate(0, -goalPaceMonths, 0))
	if err != nil {
		logger.FromCtx(ctx).Error("fetching goal contributions", zap.Error(err))
		return nil, err
	}

	inProgress := map[string]bool{}
	for i := range goals {
		inProgress[goals[i].ID] = true
	}

	paces := models.GoalPaces{}
	for _, row := range rows {
		if inProgress[row.GoalID] {
			paces[row.GoalID] = row.Amount / goalPaceMonths
		}
	}
	return paces, nil
}
