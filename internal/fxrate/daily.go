package fxrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/sharedqueries"
	"go.uber.org/zap"
)

// defaultDatedURLs are the rates of a past day (date, then base).
var defaultDatedURLs = []string{
	"https://cdn.jsdelivr.net/npm/@fawazahmed0/currency-api@%s/v1/currencies/%s.min.json",
	"https://%s.currency-api.pages.dev/v1/currencies/%s.min.json",
}

// RateOn is the rate converting from a currency to another on a day: today's
// (GetFxRate) for today or later, else that day's, fetched once and kept in
// the shared database's fx_rates_daily. Dated tells it's the day's, false
// when it fell back to today's (the day's couldn't be had).
func (j *Job) RateOn(ctx context.Context, day time.Time, from, to string) (rate float64, dated bool, err error) {
	from, to = strings.ToLower(from), strings.ToLower(to)
	if from == to {
		return 1, true, nil
	}
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if day.IsZero() || !day.Before(today) {
		rate, err := GetFxRate(from, to)
		return rate, false, err
	}

	l := logger.FromCtx(ctx)
	key := sharedqueries.GetDailyFxRateParams{Day: day, Base: from, Quote: to}
	if v, err := j.q.GetDailyFxRate(ctx, key); err == nil && v > 0 {
		return v, true, nil
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		l.Error("querying a day's fx rate", zap.Error(err))
	}

	v, err := j.fetchDayRate(ctx, day, from, to)
	if err != nil {
		l.Warn("failed to fetch a day's fx rate, using today's", zap.Error(err), zap.String("base", from), zap.String("quote", to))
		rate, err := GetFxRate(from, to)
		return rate, false, err
	}
	if err := j.q.UpsertDailyFxRate(ctx, sharedqueries.UpsertDailyFxRateParams{Day: day, Base: from, Quote: to, Rate: v}); err != nil {
		l.Error("storing a day's fx rate", zap.Error(err))
	}
	return v, true, nil
}

// fetchDayRate fetches the day's rates of from, else of the cross base to
// derive it.
func (j *Job) fetchDayRate(ctx context.Context, day time.Time, from, to string) (float64, error) {
	date := day.Format(time.DateOnly)
	if br, err := j.fetchDated(ctx, date, from, map[string]bool{to: true}); err == nil && br.rates[to] > 0 {
		return br.rates[to], nil
	}
	br, err := j.fetchDated(ctx, date, crossBase, map[string]bool{from: true, to: true})
	if err != nil {
		return 0, err
	}
	fromV, toV := br.rates[from], br.rates[to]
	if from == crossBase {
		fromV = 1
	}
	if to == crossBase {
		toV = 1
	}
	if fromV <= 0 || toV <= 0 {
		return 0, fmt.Errorf("%s: no %s → %s rate", date, from, to)
	}
	return toV / fromV, nil
}

func (j *Job) fetchDated(ctx context.Context, date, base string, wanted map[string]bool) (baseRates, error) {
	var errs []error
	for _, u := range j.datedURLs {
		br, err := j.fetchURL(ctx, fmt.Sprintf(u, date, base), base, wanted)
		if err == nil {
			return br, nil
		}
		errs = append(errs, err)
	}
	return baseRates{}, errors.Join(errs...)
}
