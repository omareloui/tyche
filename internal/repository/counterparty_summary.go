package repository

import (
	"context"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// GetCounterpartySummary is the merchant spending of the user's month named
// after month's (the current one when zero), in def, without the hidden
// merchants.
func (r *Repository) GetCounterpartySummary(ctx context.Context, month time.Time, def *models.Currency, hidden []string) (*models.CounterpartySummary, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	p := userPeriod(ctx, month)
	rows, err := q.GetMerchantDailySpending(ctx, queries.GetMerchantDailySpendingParams{
		FromDate: p.From,
		ToDate:   p.To,
	})
	if err != nil {
		logger.FromCtx(ctx).Error("querying merchant daily spending", zap.Error(err))
		return nil, err
	}

	totals := make([]models.DayMerchantTotal, 0, len(rows))
	for _, row := range rows {
		day, err := time.Parse(dayLayout, row.Day)
		if err != nil {
			continue
		}
		labels, _ := unmarshalStrings(row.CounterpartyLabels)
		totals = append(totals, models.DayMerchantTotal{
			Day: day,
			Counterparty: models.Counterparty{
				ID:         row.CounterpartyID,
				Name:       row.CounterpartyName,
				Icon:       row.CounterpartyIcon.String,
				Color:      row.CounterpartyColor.String,
				IsMerchant: true,
				Labels:     labels,
			},
			Currency: *dbCurrencyToCurrency(&row.Currency, false),
			Amount:   row.Total,
		})
	}

	cs := models.NewCounterpartySummary(p, *def, totals, hidden)
	if cs.Prev, cs.Next, err = r.periodNav(ctx, q, p, time.Now()); err != nil {
		return nil, err
	}
	return cs, nil
}
