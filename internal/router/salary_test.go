package router_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/gemini"
	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/payroll"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/taxguess"
	"github.com/omareloui/tyche/internal/valid"
)

// fakeTaxGuesser answers flat 20% rules, with a billed usage.
type fakeTaxGuesser struct {
	err    error
	calls  int
	lastIn taxguess.In
}

func (f *fakeTaxGuesser) Model() string { return "fake-flash" }

func (f *fakeTaxGuesser) Guess(ctx context.Context, in taxguess.In) (*payroll.Rules, gemini.Usage, error) {
	f.calls++
	f.lastIn = in
	usage := gemini.Usage{InputTokens: 1200, OutputTokens: 400, SearchQueries: 2}
	if f.err != nil {
		return nil, usage, f.err
	}
	return &payroll.Rules{
		Country: in.Country, Region: in.Region, Year: in.Year, Currency: in.Currency, Source: payroll.SourceAI,
		Brackets: []payroll.Bracket{{UpTo: 10_000, Rate: 0}, {UpTo: 0, Rate: 0.2}},
		Notes:    "Simplified.",
	}, usage, nil
}

func TestSalaryCalculator(t *testing.T) {
	repo, ctx, usr := setupTenant(t)
	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.DefaultCurrency, usr.Timezone = "e2e@example.com", *egp, "Africa/Cairo"
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	pair, err := jwt.NewPair(usr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fxrate.SetRates(map[string]map[string]float64{"usd": {"egp": 50, "aed": 3.6725, "eur": 0.92}}))

	newServer := func(g *fakeTaxGuesser, limit int) *httptest.Server {
		h := handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)
		h.SetReceiptOCR(nil, config.ReceiptOCR{TaxRulesPerMonth: limit})
		if g != nil {
			h.SetTaxGuess(g)
		}
		srv := httptest.NewServer(router.New(h))
		t.Cleanup(srv.Close)
		return srv
	}
	do := func(srv *httptest.Server, method, path string, form url.Values) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(form.Encode()))
		req.Header.Set("HX-Request", "true")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}
	taxScans := func() int {
		t.Helper()
		scans, err := repo.ReceiptScansSince(ctx, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, s := range scans {
			if s.Kind == models.ScanTaxRules {
				n++
			}
		}
		return n
	}

	t.Run("built in", func(t *testing.T) {
		srv := newServer(nil, 5)
		res, page := do(srv, http.MethodGet, "/tools/salary", nil)
		if res.StatusCode != http.StatusOK || !strings.Contains(page, "Salary calculator") || !strings.Contains(page, `value="EG"`) {
			t.Fatalf("status = %d, the page doesn't default to the time zone's country", res.StatusCode)
		}
		res, b := do(srv, http.MethodGet, "/tools/salary/result?country=EG&amount=20,000", nil)
		if res.StatusCode != http.StatusOK || !strings.Contains(b, "Built-in") || !strings.Contains(b, "Social insurance") {
			t.Fatalf("status = %d, body = %s", res.StatusCode, b)
		}
		// 188,494.8 a year.
		if !strings.Contains(b, "15,708") {
			t.Errorf("no take-home of 15,708 a month")
		}
		if got := res.Header.Get("HX-Replace-Url"); got != "/tools/salary?amount=20%2C000&country=EG" {
			t.Errorf("replace url = %q", got)
		}
	})

	t.Run("canada", func(t *testing.T) {
		srv := newServer(nil, 5)
		_, b := do(srv, http.MethodGet, "/tools/salary/result?country=CA&amount=100000&per=year", nil)
		for _, want := range []string{"Federal tax", "Ontario tax", "Surtax", "74,206"} {
			if !strings.Contains(b, want) {
				t.Errorf("result lacks %q", want)
			}
		}
		// Copied to own rules, the provincial tax and credits are kept.
		_, b = do(srv, http.MethodGet, "/tools/salary/rules/new?from=1&country=CA&opt_province=ab", nil)
		if !strings.Contains(b, "Alberta tax") || !strings.Contains(b, `name="extra"`) {
			t.Fatalf("copy form lacks the kept parts: %s", b)
		}
		r, _ := payroll.Builtin("CA", map[string]string{"province": "ab"})
		extra, _ := json.Marshal(map[string]any{"tax_name": r.TaxName, "credits": r.Credits, "layers": r.Layers})
		form := url.Values{
			"name": {"Alberta copy"}, "country": {"CA"}, "currency": {"CAD"}, "extra": {string(extra)},
			"bracket_up_to": {"58523", ""}, "bracket_rate": {"14", "33"},
		}
		if res, b := do(srv, http.MethodPost, "/tools/salary/rules", form); res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, body = %s", res.StatusCode, b)
		}
		rules, _ := repo.GetSalaryRules(ctx)
		var got *models.SalaryRule
		for i := range rules {
			if rules[i].Name == "Alberta copy" {
				got = &rules[i]
			}
		}
		if got == nil || len(got.Rules.Layers) != 1 || got.Rules.Layers[0].Name != "Alberta" || len(got.Rules.Credits) != 2 {
			t.Fatalf("saved = %+v", got)
		}
		repo.DeleteSalaryRule(ctx, got.ID)
	})

	t.Run("net to gross", func(t *testing.T) {
		srv := newServer(nil, 5)
		_, b := do(srv, http.MethodGet, "/tools/salary/result?country=US&dir=net&per=year&amount=79180&opt_filing=single", nil)
		if !strings.Contains(b, "100,000") {
			t.Errorf("79,180 net isn't 100,000 gross: %s", b)
		}
	})

	t.Run("ai", func(t *testing.T) {
		g := &fakeTaxGuesser{}
		srv := newServer(g, 2)
		_, b := do(srv, http.MethodGet, "/tools/salary/result?country=FR&amount=3000", nil)
		if !strings.Contains(b, "Look up the rules") || !strings.Contains(b, "2 left this month") {
			t.Fatalf("no lookup offered: %s", b)
		}
		form := url.Values{"country": {"FR"}, "amount": {"3000"}, "rules": {"ai"}}
		res, b := do(srv, http.MethodPost, "/tools/salary/rules/fetch", form)
		if res.StatusCode != http.StatusOK || !strings.Contains(b, "AI lookup") || g.calls != 1 {
			t.Fatalf("status = %d, calls = %d, body = %s", res.StatusCode, g.calls, b)
		}
		if g.lastIn.Currency != "EUR" || g.lastIn.Name != "France" {
			t.Errorf("asked %+v", g.lastIn)
		}
		// 36,000 a year less 20% over 10,000.
		if !strings.Contains(b, "2,567") {
			t.Errorf("no take-home of 2,567 a month: %s", b)
		}
		// Kept: no call, nothing counted.
		do(srv, http.MethodPost, "/tools/salary/rules/fetch", form)
		_, b = do(srv, http.MethodGet, "/tools/salary/result?country=FR&amount=3000", nil)
		if g.calls != 1 || taxScans() != 1 || !strings.Contains(b, "AI lookup") {
			t.Errorf("calls = %d, scans = %d", g.calls, taxScans())
		}
		// Asked again: counted, then the limit.
		form.Set("refresh", "1")
		do(srv, http.MethodPost, "/tools/salary/rules/fetch", form)
		_, b = do(srv, http.MethodPost, "/tools/salary/rules/fetch", form)
		if g.calls != 2 || taxScans() != 2 || !strings.Contains(b, "the most you can") {
			t.Errorf("calls = %d, scans = %d, body = %s", g.calls, taxScans(), b)
		}
		// A region is its own lookup.
		g2 := &fakeTaxGuesser{err: errors.New("gemini answered 503")}
		_, b = do(newServer(g2, 5), http.MethodPost, "/tools/salary/rules/fetch", url.Values{"country": {"FR"}, "region": {"Corsica"}, "rules": {"ai"}})
		if g2.calls != 1 || !strings.Contains(b, "try again in a bit") {
			t.Errorf("calls = %d, body = %s", g2.calls, b)
		}
	})

	t.Run("own rules", func(t *testing.T) {
		srv := newServer(nil, 5)
		bad := url.Values{"name": {"Mine"}, "country": {"DE"}, "currency": {"EUR"}, "bracket_up_to": {"20000", "10000"}, "bracket_rate": {"10", "20"}}
		if res, b := do(srv, http.MethodPost, "/tools/salary/rules", bad); res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(b, "go up") {
			t.Fatalf("status = %d, body = %s", res.StatusCode, b)
		}
		form := url.Values{
			"name": {"Germany (mine)"}, "country": {"DE"}, "currency": {"EUR"}, "pays": {"12"},
			"bracket_up_to": {"12000", ""}, "bracket_rate": {"0", "30"},
			"contrib_name": {"Social"}, "contrib_rate": {"20"}, "contrib_floor": {""}, "contrib_cap": {""},
			"contrib_monthly": {""}, "contrib_deductible": {""}, "contrib_bands": {""},
		}
		res, _ := do(srv, http.MethodPost, "/tools/salary/rules", form)
		var trig struct {
			Changed struct{ ID string } `json:"salary-rules-changed"`
		}
		if err := json.Unmarshal([]byte(res.Header.Get("HX-Trigger")), &trig); err != nil || trig.Changed.ID == "" {
			t.Fatalf("status = %d, trigger = %q", res.StatusCode, res.Header.Get("HX-Trigger"))
		}
		id := trig.Changed.ID
		rules, err := repo.GetSalaryRules(ctx)
		if err != nil || len(rules) != 1 || rules[0].Rules.Contributions[0].Rate != 0.2 {
			t.Fatalf("rules = %+v, %v", rules, err)
		}
		// 60,000: 12,000 social, (60,000 − 12,000) × 30% tax → 33,600 a year.
		_, b := do(srv, http.MethodGet, "/tools/salary/result?rules="+id+"&amount=60000&per=year", nil)
		if !strings.Contains(b, "Germany (mine)") || !strings.Contains(b, "2,800") {
			t.Errorf("body = %s", b)
		}
		if res, _ := do(srv, http.MethodDelete, "/tools/salary/rules/"+id, nil); res.StatusCode != http.StatusOK {
			t.Errorf("delete status = %d", res.StatusCode)
		}
		// Gone: the country's AI rules instead.
		_, b = do(srv, http.MethodGet, "/tools/salary/result?rules="+id+"&country=DE&amount=60000", nil)
		if strings.Contains(b, "Germany (mine)") {
			t.Error("deleted rules still used")
		}
	})

	t.Run("save net", func(t *testing.T) {
		srv := newServer(nil, 5)
		res, b := do(srv, http.MethodPut, "/tools/salary/save-net", url.Values{"country": {"EG"}, "amount": {"20000"}})
		if res.StatusCode != http.StatusOK || !strings.Contains(b, "Saved") {
			t.Fatalf("status = %d, body = %s", res.StatusCode, b)
		}
		ws, err := repo.GetWorkSettings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(ws.MonthlySalary-15_707.9) > 0.01 || ws.Currency.String != "EGP" || ws.HoursPerWeek <= 0 {
			t.Errorf("work settings = %+v", ws)
		}
	})

	t.Run("compare", func(t *testing.T) {
		srv := newServer(nil, 5)
		res, b := do(srv, http.MethodGet, "/tools/salary/compare/result?n=2&o0_country=EG&o0_amount=20000&o1_country=AE&o1_amount=10000", nil)
		if res.StatusCode != http.StatusOK || !strings.Contains(b, "Offer 2") {
			t.Fatalf("status = %d, body = %s", res.StatusCode, b)
		}
		// 10,000 AED tax free ≈ 136,147 EGP, way over Egypt's.
		if i, j := strings.Index(b, "Takes home the most"), strings.Index(b, "Offer 2"); i < j {
			t.Errorf("the best isn't the second offer")
		}
		// An offer as a take-home pay a year: the gross it needs.
		res, b = do(srv, http.MethodGet, "/tools/salary/compare/result?n=2&o0_country=US&o0_opt_filing=single&o0_dir=net&o0_per=year&o0_amount=79180&o1_country=AE&o1_amount=10000", nil)
		if !strings.Contains(b, "Needs a gross of") || !strings.Contains(b, "100,000 a year") {
			t.Errorf("no gross for the net offer: %s", b)
		}
		if u := res.Header.Get("HX-Replace-Url"); !strings.Contains(u, "o0_dir=net") || !strings.Contains(u, "o0_per=year") {
			t.Errorf("replace url = %q", u)
		}
		// Bonus and other tax per offer: 10% of 100,000 at 22% + 7.65%, 5%
		// other on 110,000; with the conversion's tooltip.
		_, b = do(srv, http.MethodGet, "/tools/salary/compare/result?n=2&o0_country=US&o0_per=year&o0_amount=100000&o0_bonus=10&o0_bonus_in=pct&o0_other=5&o1_country=EG&o1_amount=20000", nil)
		// (79,180 + 7,035 − 5,500) / 12 = 6,726.
		if !strings.Contains(b, "6,726") || !strings.Contains(b, "the bonus spread over the year") || !strings.Contains(b, "at today&#39;s exchange rate") {
			t.Errorf("offer with bonus: %s", b)
		}
		_, b = do(srv, http.MethodGet, "/tools/salary/compare/result?n=3&remove=0&o0_country=EG&o1_country=AE&o2_country=GB", nil)
		if strings.Contains(b, "Offer 3") {
			t.Error("offer not removed")
		}
	})
}

func TestSalaryBonusPercent(t *testing.T) {
	repo, ctx, usr := setupTenant(t)
	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.DefaultCurrency = "e2e@example.com", *egp
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	pair, err := jwt.NewPair(usr)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)))
	t.Cleanup(srv.Close)
	get := func(path string) string {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		req.Header.Set("HX-Request", "true")
		req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return string(b)
	}
	base := "/tools/salary/result?country=US&opt_filing=single&per=year&amount=100000"
	byPct := get(base + "&bonus=10&bonus_in=pct")
	byAmount := get(base + "&bonus=10000")
	// 10,000 at 22% + 7.65%: 7,035 kept.
	for _, b := range []string{byPct, byAmount} {
		if !strings.Contains(b, "7,035") {
			t.Errorf("bonus take-home missing: %s", b)
		}
	}
	// Net → gross a month: the hero's big amount is the month's, the year
	// under it.
	hero := get("/tools/salary/result?country=US&opt_filing=single&dir=net&amount=6598.34")
	if m, y := strings.Index(hero, "8,333"), strings.Index(hero, "100,000"); m < 0 || y < 0 || m > y {
		t.Errorf("month at %d, year at %d", m, y)
	}
	// Income tax as a share of the gross: 15,370 of 110,000.
	if !strings.Contains(byPct, "14.0%") {
		t.Error("no income tax share")
	}
}
