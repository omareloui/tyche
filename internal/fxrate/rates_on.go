package fxrate

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/sharedqueries"
	"go.uber.org/zap"
)

// ratesOnFetches is how many days' rates RatesOn fetches at once.
const ratesOnFetches = 4

// RatesOn is RateOn of each of codes to `to` on each of days, by day (as
// given) then code: the past days' read from fx_rates_daily, the missing ones
// fetched with one dated request of `to` per day (its rates to every code,
// inverted), a few days at once, then stored; a rate that can't be had that
// way goes through RateOn. Approx tells some fell back to today's rates.
func (j *Job) RatesOn(ctx context.Context, days []time.Time, codes []string, to string) (rates map[time.Time]map[string]float64, approx bool) {
	l := logger.FromCtx(ctx)
	to = strings.ToLower(to)
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	rates = make(map[time.Time]map[string]float64, len(days))
	// The past days missing some rates, with the codes they miss.
	missing := map[time.Time][]string{}
	for _, d := range days {
		day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
		dr := make(map[string]float64, len(codes))
		rates[d] = dr
		for _, c := range codes {
			c = strings.ToLower(c)
			if c == to {
				dr[c] = 1
				continue
			}
			if !day.Before(today) {
				v, err := GetFxRate(c, to)
				if err != nil {
					approx = true
					continue
				}
				dr[c] = v
				continue
			}
			v, err := j.q.GetDailyFxRate(ctx, sharedqueries.GetDailyFxRateParams{Day: day, Base: c, Quote: to})
			if err == nil && v > 0 {
				dr[c] = v
				continue
			}
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				l.Error("querying a day's fx rate", zap.Error(err))
			}
			// The other way round, e.g. kept by the fx job's fetch of `to`.
			if v, err := j.q.GetDailyFxRate(ctx, sharedqueries.GetDailyFxRateParams{Day: day, Base: to, Quote: c}); err == nil && v > 0 {
				dr[c] = 1 / v
				continue
			}
			missing[d] = append(missing[d], c)
		}
	}
	if len(missing) == 0 {
		return rates, approx
	}

	// Fetched a few days at once; written one at a time after.
	type fetched struct {
		day   time.Time
		rates map[string]float64
	}
	var (
		mu      sync.Mutex
		results []fetched
		wg      sync.WaitGroup
		sem     = make(chan struct{}, ratesOnFetches)
	)
	for d, cs := range missing {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			wanted := make(map[string]bool, len(cs))
			for _, c := range cs {
				wanted[c] = true
			}
			br, err := j.fetchDated(ctx, d.Format(time.DateOnly), to, wanted)
			if err != nil {
				l.Warn("failed to fetch a day's fx rates", zap.Error(err), zap.String("base", to))
				return
			}
			mu.Lock()
			results = append(results, fetched{d, br.rates})
			mu.Unlock()
		}()
	}
	wg.Wait()

	got := make(map[time.Time]map[string]float64, len(results))
	for _, f := range results {
		got[f.day] = f.rates
	}
	for d, cs := range missing {
		day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
		for _, c := range cs {
			if v := got[d][c]; v > 0 {
				rate := 1 / v
				rates[d][c] = rate
				if err := j.q.UpsertDailyFxRate(ctx, sharedqueries.UpsertDailyFxRateParams{Day: day, Base: c, Quote: to, Rate: rate}); err != nil {
					l.Error("storing a day's fx rate", zap.Error(err))
				}
				continue
			}
			v, dated, err := j.RateOn(ctx, day, c, to)
			if err != nil {
				approx = true
				continue
			}
			if !dated {
				approx = true
			}
			rates[d][c] = v
		}
	}
	return rates, approx
}
