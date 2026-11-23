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
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	from, to := models.HeatmapPeriod(monthStartFromCtx(ctx), time.Now(), months)
	rows, err := q.GetSpendingTimes(ctx, queries.GetSpendingTimesParams{FromDate: from, ToDate: to})
	if err != nil {
		logger.FromCtx(ctx).Error("querying spending times", zap.Error(err))
		return nil, err
	}
	txs, partial := spendingTimes(rows, def)
	return models.NewSpendingHeatmap(*def, metric, months, from, to, weekStart, loc, txs, partial), nil
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
		txs = append(txs, models.SpendingTime{At: row.ScheduledTo, Amount: amount})
	}
	return txs, partial
}
