package fxrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/sharedqueries"
	_ "github.com/omareloui/tyche/internal/sqlite"
)

func TestGetFxRate(t *testing.T) {
	setRates(rates{
		"usd": {"eur": 0.5, "egp": 50},
		"eur": {"usd": 2},
	})

	tests := []struct {
		from, to string
		want     float64
		err      error
	}{
		{"USD", "usd", 1, nil},
		{"usd", "eur", 0.5, nil},
		{"EUR", "USD", 2, nil},
		{"egp", "eur", 0.01, nil}, // via usd
		{"usd", "xyz", 0, ErrNoRate},
	}
	for _, tt := range tests {
		got, err := GetFxRate(tt.from, tt.to)
		if !errors.Is(err, tt.err) || got != tt.want {
			t.Errorf("GetFxRate(%q, %q) = %v, %v; want %v, %v", tt.from, tt.to, got, err, tt.want, tt.err)
		}
	}
}

// newTestJob makes a job over a fresh shared database, fetching from srv.
func newTestJob(t *testing.T, srv *httptest.Server, codes []string) *Job {
	t.Helper()

	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "shared.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	schema, err := os.ReadFile("../../sql/shared/schema/20261031120000_fx_rates.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.Split(string(schema), "-- +goose Down")[0]
	if _, err := db.Exec(up); err != nil {
		t.Fatal(err)
	}

	j := NewJob(db, sharedqueries.New(db), codes, time.Hour)
	j.urls = []string{srv.URL + "/primary/%s.json", srv.URL + "/fallback/%s.json"}
	return j
}

func TestRunOnce(t *testing.T) {
	var failEUR atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/primary/usd.json":
			fmt.Fprint(w, `{"date":"2026-10-30","usd":{"eur":0.5,"egp":50,"usd":1,"btc":0.00001}}`)
		case "/primary/eur.json":
			http.Error(w, "down", http.StatusBadGateway)
		case "/fallback/eur.json":
			if failEUR.Load() {
				http.Error(w, "down", http.StatusBadGateway)
				return
			}
			fmt.Fprint(w, `{"date":"2026-10-30","eur":{"usd":2,"egp":100}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	j := newTestJob(t, srv, []string{"USD", "EUR", "EGP"})

	if err := j.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	if got, _ := GetFxRate("eur", "egp"); got != 100 {
		t.Errorf("eur → egp = %v, want 100 (from the fallback url)", got)
	}
	if _, err := GetFxRate("usd", "btc"); !errors.Is(err, ErrNoRate) {
		t.Errorf("usd → btc should be filtered out, got err %v", err)
	}

	st := j.Status()
	if st.Running || st.LastErr != "" || len(st.FailedBases) != 1 || st.FailedBases[0] != "egp" {
		t.Errorf("unexpected status %+v", st)
	}

	info, err := j.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// usd: eur, egp; eur: usd, egp.
	if info.Pairs != 4 || info.Bases != 2 || info.AsOf != "2026-10-30" || info.FetchedAt.IsZero() {
		t.Errorf("unexpected info %+v", info)
	}

	// A failing base keeps its stored rates.
	failEUR.Store(true)
	setRates(nil)
	if err := j.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := GetFxRate("eur", "egp"); got != 100 {
		t.Errorf("eur → egp = %v after a failed fetch, want the stale 100", got)
	}
}

func TestRunOnceAllFail(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	j := newTestJob(t, srv, []string{"usd"})
	if err := j.RunOnce(context.Background()); err == nil {
		t.Fatal("expected an error when every base fails")
	}
	if st := j.Status(); st.LastErr == "" || !st.LastSuccessAt.IsZero() {
		t.Errorf("unexpected status %+v", st)
	}
}

func TestTrigger(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	j := newTestJob(t, srv, []string{"usd"})
	if !j.Trigger() {
		t.Fatal("first trigger should be queued")
	}
	if j.Trigger() {
		t.Fatal("second trigger should be dropped while one is queued")
	}
}
