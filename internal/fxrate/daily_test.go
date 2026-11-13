package fxrate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateOn(t *testing.T) {
	var hits atomic.Int32
	fail := atomic.Bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if fail.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case "/2026-11-01/eur.json":
			_, _ = w.Write([]byte(`{"date":"2026-11-01","eur":{"egp":53.31,"usd":1.1}}`))
		case "/2026-11-01/usd.json":
			_, _ = w.Write([]byte(`{"date":"2026-11-01","usd":{"egp":48,"gbp":0.8}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	j := newTestJob(t, srv, []string{"eur", "egp"})
	schema, err := os.ReadFile("../../sql/shared/schema/20261118000000_fx_rates_daily.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.db.Exec(strings.Split(strings.Split(string(schema), "-- +goose Down")[0], "-- +goose Up")[1]); err != nil {
		t.Fatal(err)
	}
	j.datedURLs = []string{srv.URL + "/%s/%s.json"}
	defer SetRates(map[string]map[string]float64{"eur": {"egp": 50}, "usd": {"egp": 47, "gbp": 0.75, "eur": 0.9}})()

	ctx := context.Background()
	day := time.Date(2026, 11, 1, 15, 0, 0, 0, time.UTC)

	if got, dated, err := j.RateOn(ctx, day, "EUR", "EGP"); err != nil || !dated || got != 53.31 {
		t.Fatalf("RateOn = %v, %v, %v; want the day's 53.31", got, dated, err)
	}
	n := hits.Load()
	if got, _, _ := j.RateOn(ctx, day, "eur", "egp"); got != 53.31 || hits.Load() != n {
		t.Errorf("second call = %v with %d fetches, want it from fx_rates_daily", got, hits.Load()-n)
	}

	// Not in the base's rates: across the usd's.
	if got, dated, _ := j.RateOn(ctx, day, "gbp", "egp"); !dated || got != 48/0.8 {
		t.Errorf("cross rate = %v (dated %v), want %v", got, dated, 48/0.8)
	}

	// Today's comes from memory, a failed fetch falls back to it.
	if got, dated, _ := j.RateOn(ctx, time.Now(), "eur", "egp"); dated || got != 50 {
		t.Errorf("today = %v (dated %v), want 50", got, dated)
	}
	fail.Store(true)
	if got, dated, _ := j.RateOn(ctx, day.AddDate(0, 0, -3), "eur", "egp"); dated || got != 50 {
		t.Errorf("failed fetch = %v (dated %v), want today's 50", got, dated)
	}
}
