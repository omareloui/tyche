package handler

import (
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/web/views"
)

func TestAccountsSummaryIn(t *testing.T) {
	e := newPickEnv(t)
	e.expense(t, nil)
	def := getClaims(e.ctx).DefaultCurrency

	r := httptest.NewRequest("GET", "/accounts/summary", nil).WithContext(e.ctx)
	s, err := e.h.accountsSummary(r)
	if err != nil {
		t.Fatal(err)
	}
	if s.In != nil || s.HistoryIn != nil || len(s.InChoices) == 0 {
		t.Fatalf("in = %v, choices %v; want the default with choices", s.In, s.InChoices)
	}
	for _, c := range s.InChoices {
		if strings.EqualFold(c.String, def.String) {
			t.Errorf("the default currency %s is a choice", def.String)
		}
	}

	target := s.InChoices[0]
	t.Cleanup(fxrate.SetRates(map[string]map[string]float64{
		strings.ToLower(def.String): {strings.ToLower(target.String): 0.5},
	}))
	r = httptest.NewRequest("GET", "/accounts/summary?in="+strings.ToLower(target.String), nil).WithContext(e.ctx)
	s, err = e.h.accountsSummary(r)
	if err != nil {
		t.Fatal(err)
	}
	if s.In == nil || s.In.String != target.String || s.HistoryIn == nil {
		t.Fatalf("in = %v, want %s", s.In, target.String)
	}
	// Without the fx job, every day is at today's rates.
	if !s.InApprox {
		t.Error("not approximate without dated rates")
	}
	i := s.History.TodayIdx()
	if got, want := s.HistoryIn.Balance[i], s.History.Balance[i]*0.5; math.Abs(got-want) > 1e-9 {
		t.Errorf("today in %s = %v, want %v", target.String, got, want)
	}

	var b strings.Builder
	if err := views.AccountsSummary(s).Render(e.ctx, &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `data-acc-summary-in="`+target.String+`" aria-pressed="true"`) {
		t.Error("the picked currency isn't pressed")
	}
	b.Reset()
	if err := views.DashNetWorth(s).Render(e.ctx, &b); err != nil {
		t.Fatal(err)
	}

	// Not a choice: left in the default currency.
	r = httptest.NewRequest("GET", "/accounts/summary?in=JPY", nil).WithContext(e.ctx)
	if s, err = e.h.accountsSummary(r); err != nil || s.In != nil {
		t.Errorf("in=JPY: %v, %v; want the default", s.In, err)
	}
}

func TestAccountsSummaryRange(t *testing.T) {
	e := newPickEnv(t)
	old := time.Now().UTC().AddDate(-2, -1, 0)
	if err := e.repo.CreateTransaction(e.ctx, &models.Transaction{
		ID: idgen.New(), Account: e.acc, Amount: 10, Kind: models.ExpenseTxKind,
		ScheduledTo: old, FulfilledAt: old,
	}, nil, nil); err != nil {
		t.Fatal(err)
	}

	summary := func(query string) *models.AccountsSummary {
		t.Helper()
		r := httptest.NewRequest("GET", "/accounts/summary?"+query, nil).WithContext(e.ctx)
		s, err := e.h.accountsSummary(r)
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		if err := views.AccountsSummary(s).Render(e.ctx, &b); err != nil {
			t.Fatal(err)
		}
		return s
	}

	s := summary("")
	if s.History.Months != models.AccountHistoryMonths {
		t.Errorf("default months = %d, want %d", s.History.Months, models.AccountHistoryMonths)
	}
	s = summary("months=12")
	if want := models.AccountHistoryFromMonths(models.MonthStart(1), s.History.Today, 12); s.History.Months != 12 || !s.History.From.Equal(want) {
		t.Errorf("12 months: from %v (%d months), want %v", s.History.From, s.History.Months, want)
	}
	s = summary("months=7") // not a choice
	if s.History.Months != models.AccountHistoryMonths {
		t.Errorf("months=7 = %d months, want the default", s.History.Months)
	}
	s = summary("months=all")
	if want := models.MonthStart(1).Of(old).From; s.History.Months != 0 || !s.History.From.Equal(want) {
		t.Errorf("all: from %v (%d months), want %v", s.History.From, s.History.Months, want)
	}
	// Two years of days: the rates are sampled wider than a week.
	if days := s.RateDays(); len(days) > 61 {
		t.Errorf("%d rate days, want 61 at most", len(days))
	}
}
