package repository

import (
	"context"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// GetSpendingHeatmap is the spending of the last months user months (the
// current one included) per weekday and hour of loc, in def at today's
// rates.
func (r *Repository) GetSpendingHeatmap(ctx context.Context, months int, metric models.HeatmapMetric, def *models.Currency, loc *time.Location, weekStart time.Weekday) (*models.SpendingHeatmap, error) {
	sp, err := r.getHeatmapSpending(ctx, months, def)
	if err != nil {
		return nil, err
	}
	hm := models.NewSpendingHeatmap(*def, metric, months, sp.from, sp.to, weekStart, loc, sp.txs, sp.partial)
	hm.SetCategories(sp.cats)
	return hm, nil
}

// GetMonthDaysHeatmap is the spending of the last months user months (the
// current one included) per day of each, a day being loc's, in def at
// today's rates.
func (r *Repository) GetMonthDaysHeatmap(ctx context.Context, months int, metric models.HeatmapMetric, def *models.Currency, loc *time.Location) (*models.MonthDaysHeatmap, error) {
	sp, err := r.getHeatmapSpending(ctx, months, def)
	if err != nil {
		return nil, err
	}
	hm := models.NewMonthDaysHeatmap(monthStartFromCtx(ctx), *def, metric, months, sp.from, sp.to, r.userToday(ctx), loc, sp.txs, sp.partial)
	hm.SetCategories(sp.cats)
	return hm, nil
}

// heatmapSpending is what both heatmaps are made of.
type heatmapSpending struct {
	from, to time.Time
	txs      []models.SpendingTime
	partial  bool
	cats     []models.Category
}

// getHeatmapSpending is the spending of the last months user months, in
// def, and the categories naming it.
func (r *Repository) getHeatmapSpending(ctx context.Context, months int, def *models.Currency) (*heatmapSpending, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	sp := &heatmapSpending{}
	sp.from, sp.to = models.HeatmapPeriod(monthStartFromCtx(ctx), r.userToday(ctx), months)
	rows, err := q.GetSpendingTimes(ctx, queries.GetSpendingTimesParams{FromDate: sp.from, ToDate: sp.to})
	if err != nil {
		logger.FromCtx(ctx).Error("querying spending times", zap.Error(err))
		return nil, err
	}
	sp.txs, sp.partial = spendingTimes(rows, def)
	if sp.cats, err = r.GetCategories(ctx); err != nil {
		return nil, err
	}
	return sp, nil
}

// spendingTimes are rows in def, partial when a currency had no rate.
func spendingTimes(rows []queries.GetSpendingTimesRow, def *models.Currency) ([]models.SpendingTime, bool) {
	txs := make([]models.SpendingTime, 0, len(rows))
	var partial bool
	for _, row := range rows {
		amount, ok := models.Convert(row.Amount, dbCurrencyToCurrency(&row.Currency, false), def.String)
		if !ok {
			partial = true
			continue
		}
		txs = append(txs, models.SpendingTime{At: row.ScheduledTo, CategoryID: row.CategoryID, Amount: amount})
	}
	return txs, partial
}
