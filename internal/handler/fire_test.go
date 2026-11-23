package handler

import (
	"math"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/web/views"
)

func TestFireCalc(t *testing.T) {
	e := newPickEnv(t)
	// Last month: 3,000 in, 1,000 out.
	now := time.Now().UTC()
	day := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 4)
	for _, tx := range []struct {
		kind   models.TxKind
		amount float64
	}{{models.IncomeTxKind, 3_000}, {models.ExpenseTxKind, 1_000}} {
		if err := e.repo.CreateTransaction(e.ctx, &models.Transaction{
			ID: idgen.New(), Account: e.acc, Amount: tx.amount, Kind: tx.kind, ScheduledTo: day, FulfilledAt: day,
		}, nil, nil); err != nil {
			t.Fatal(err)
		}
	}

	data := func(q url.Values) *views.FireData {
		t.Helper()
		d, err := e.h.fireData(e.ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}

	d := data(url.Values{})
	if d.Error != "" || d.Result == nil || d.Sim == nil {
		t.Fatalf("no result: %q", d.Error)
	}
	if d.Money.Months() != 1 || d.SpendingDef != 1_000 || d.SavingDef != 2_000 || d.SavingFrom != views.FireSavingFromFlow {
		t.Fatalf("defaults: months %d spending %v saving %v from %q", d.Money.Months(), d.SpendingDef, d.SavingDef, d.SavingFrom)
	}
	if d.PortfolioDef != 2_000 {
		t.Errorf("portfolio = %v, want 2000", d.PortfolioDef)
	}
	if d.Result.Number != 300_000 || d.Input.SWR != 0.04 || d.Input.Return != 0.05 {
		t.Errorf("number %v swr %v return %v", d.Result.Number, d.Input.SWR, d.Input.Return)
	}
	// Without a birth year: no ages, no coast.
	for _, v := range d.Result.Variants {
		if v.Key == "coast" {
			t.Error("coast without an age")
		}
	}

	// Typed spending: the saving follows it.
	d = data(url.Values{"spending": {"1,500"}, "swr": {"3.5"}, "return": {"-1"}})
	if d.SavingDef != 1_500 || d.Input.Spending != 1_500 || math.Abs(d.Input.SWR-0.035) > 1e-12 || math.Abs(d.Input.Return+0.01) > 1e-12 {
		t.Errorf("typed: saving %v spending %v swr %v return %v", d.SavingDef, d.Input.Spending, d.Input.SWR, d.Input.Return)
	}
	// A typed 0 saving is kept.
	if d = data(url.Values{"saving": {"0"}}); d.Input.Saving != 0 {
		t.Errorf("saving 0 = %v", d.Input.Saving)
	}

	// Errors.
	if d = data(url.Values{"swr": {"20"}}); !strings.Contains(d.Error, "withdrawal") {
		t.Errorf("swr 20%%: %q", d.Error)
	}
	if d = data(url.Values{"return": {"abc"}}); !strings.Contains(d.Error, "return") {
		t.Errorf("bad return: %q", d.Error)
	}

	// The birth year gives ages and the coast.
	if err := e.repo.SetUserProfile(e.ctx, "Pick", "Er", now.Year()-30); err != nil {
		t.Fatal(err)
	}
	d = data(url.Values{"retire_age": {"60"}})
	if d.Age != 30 || d.Input.RetireAge != 60 {
		t.Fatalf("age %d retire %d", d.Age, d.Input.RetireAge)
	}
	coast := false
	for _, v := range d.Result.Variants {
		coast = coast || v.Key == "coast"
	}
	if !coast || d.Result.Path[0].Age != 30 {
		t.Error("no coast or ages with a birth year")
	}

	if u := views.FireURL(views.FireForm{Spending: "1,500", Goals: true}); u != "/tools/fire?goals=1&spending=1%2C500" {
		t.Errorf("url = %s", u)
	}
	if u := views.FireURL(views.FireForm{}); u != "/tools/fire" {
		t.Errorf("url = %s", u)
	}
}

func TestParseBirthYear(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for s, want := range map[string]struct {
		y  int
		ok bool
	}{"": {0, true}, "1990": {1990, true}, "1899": {0, false}, "2027": {0, false}, "abc": {0, false}} {
		if y, ok := parseBirthYear(s, now); y != want.y || ok != want.ok {
			t.Errorf("%q = %d %v", s, y, ok)
		}
	}
}
