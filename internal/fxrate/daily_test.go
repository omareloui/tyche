package fxrate

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestRatesOn(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/2026-10-01/usd.json":
			_, _ = w.Write([]byte(`{"date":"2026-10-01","usd":{"egp":40,"eur":0.8,"xau":0.0005}}`))
		case "/2026-10-08/usd.json":
			_, _ = w.Write([]byte(`{"date":"2026-10-08","usd":{"egp":50,"eur":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	j := newTestJob(t, srv, []string{"usd", "egp", "eur"})
	j.datedURLs = []string{srv.URL + "/%s/%s.json"}
	defer SetRates(map[string]map[string]float64{
		"usd": {"egp": 47, "eur": 0.9, "xau": 0.0004},
		"egp": {"usd": 1.0 / 47}, "xau": {"usd": 2500},
	})()

	ctx := context.Background()
	d1 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	today := time.Now().UTC()
	days := []time.Time{d1, d2, today}

	rates, approx := j.RatesOn(ctx, days, []string{"EGP", "XAU", "USD"}, "USD")
	if got := rates[d1]["egp"]; got != 1.0/40 {
		t.Errorf("egp on d1 = %v, want 1/40", got)
	}
	if got := rates[d1]["xau"]; got != 1/0.0005 {
		t.Errorf("xau on d1 = %v, want 2000", got)
	}
	if got := rates[d1]["usd"]; got != 1 {
		t.Errorf("usd = %v, want 1", got)
	}
	if got := rates[d2]["egp"]; got != 1.0/50 {
		t.Errorf("egp on d2 = %v, want 1/50", got)
	}
	// d2 lacks xau: today's, approximate.
	if !approx || rates[d2]["xau"] != 1/0.0004 {
		t.Errorf("xau on d2 = %v (approx %v), want today's 2500 approximate", rates[d2]["xau"], approx)
	}
	if got := rates[today]["egp"]; got != 1.0/47 {
		t.Errorf("egp today = %v, want today's 1/47", got)
	}

	// Asked again: from fx_rates_daily but d2's xau (not stored, refetched).
	n := hits.Load()
	rates, _ = j.RatesOn(ctx, []time.Time{d1}, []string{"egp", "xau"}, "usd")
	if hits.Load() != n || rates[d1]["egp"] != 1.0/40 {
		t.Errorf("second call: %d fetches, egp %v; want none, from the cache", hits.Load()-n, rates[d1]["egp"])
	}
}
