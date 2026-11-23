package router_test

import (
	"context"
	"encoding/json"
	"io"
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
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
	"github.com/omareloui/tyche/internal/visa"
	"github.com/omareloui/tyche/internal/visaguess"
)

// fakeVisaGuesser answers a golden visa of a 250,000 investment.
type fakeVisaGuesser struct {
	calls  int
	lastIn visaguess.In
}

func (f *fakeVisaGuesser) Model() string { return "fake-flash" }

func (f *fakeVisaGuesser) Guess(ctx context.Context, in visaguess.In) (*visa.Rule, gemini.Usage, error) {
	f.calls++
	f.lastIn = in
	return &visa.Rule{
		Country: in.Country, Key: "golden-visa", Name: "Golden Visa", Category: visa.CategoryInvestor,
		Year: in.Year, Currency: "EUR", Source: visa.SourceAI,
		Requirements: []visa.Requirement{
			{Kind: visa.KindInvestment, Period: visa.PeriodOnce, Amount: 250_000},
			{Kind: visa.KindCheck, Text: "Clean record"},
		},
	}, gemini.Usage{InputTokens: 900, OutputTokens: 300, SearchQueries: 1}, nil
}

func TestVisaReadiness(t *testing.T) {
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
	// 1 EGP = 0.0184 EUR: 150,000 EGP = 2,760 EUR.
	t.Cleanup(fxrate.SetRates(map[string]map[string]float64{"usd": {"egp": 50, "eur": 0.92, "cad": 1.4, "thb": 33}}))

	kinds, err := repo.GetAccountKinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var bankKind models.AccountKind
	for _, k := range kinds {
		if k.Enum == models.AccKindBankAcc {
			bankKind = k
		}
	}
	seeded, err := repo.GetAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range seeded {
		a.IsArchive = true
		if err := repo.UpdateAccount(ctx, &a); err != nil {
			t.Fatal(err)
		}
	}
	// 500,000 EGP saved (9,200 EUR) once the incomes are in.
	bank := &models.Account{ID: idgen.New(), Name: "Bank", Amount: 500_000 - 12*150_000, Currency: *egp, Kind: bankKind}
	if err := repo.CreateAccount(ctx, bank); err != nil {
		t.Fatal(err)
	}
	// 150,000 EGP in each of the last 12 full months.
	now := time.Now().UTC()
	first := time.Date(now.Year(), now.Month(), 15, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 12; i++ {
		day := first.AddDate(0, -i, 0)
		tx := &models.Transaction{ID: idgen.New(), Account: *bank, Amount: 150_000, Kind: models.IncomeTxKind, ScheduledTo: day, FulfilledAt: day}
		if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
			t.Fatal(err)
		}
	}

	newServer := func(g *fakeVisaGuesser, limit int) *httptest.Server {
		h := handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)
		h.SetReceiptOCR(nil, config.ReceiptOCR{VisaRulesPerMonth: limit})
		if g != nil {
			h.SetVisaGuess(g)
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
	visaScans := func() int {
		t.Helper()
		scans, err := repo.ReceiptScansSince(ctx, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, s := range scans {
			if s.Kind == models.ScanVisaRules {
				n++
			}
		}
		return n
	}

	t.Run("defaults", func(t *testing.T) {
		srv := newServer(nil, 5)
		res, page := do(srv, http.MethodGet, "/tools/visa", nil)
		// Egypt has no built-in visas: the AI's, asking a name.
		if res.StatusCode != http.StatusOK || !strings.Contains(page, `value="EG"`) || !strings.Contains(page, "Type the visa&#39;s name") {
			t.Fatalf("status = %d, page = %s", res.StatusCode, page)
		}
		_, page = do(srv, http.MethodGet, "/tools", nil)
		if !strings.Contains(page, "/tools/visa") {
			t.Error("no tools card")
		}
	})

	t.Run("from transactions", func(t *testing.T) {
		srv := newServer(nil, 5)
		res, b := do(srv, http.MethodGet, "/tools/visa/result?country=ES&visa=digital-nomad", nil)
		// 2,760 EUR on average against 2,849.
		if res.StatusCode != http.StatusOK || !strings.Contains(b, "Not yet") || !strings.Contains(b, "on average") || !strings.Contains(b, "2,849") {
			t.Fatalf("status = %d, body = %s", res.StatusCode, b)
		}
		if got := res.Header.Get("HX-Replace-Url"); got != "/tools/visa?country=ES&visa=digital-nomad" {
			t.Errorf("replace url = %q", got)
		}
		// Typed: 160,000 EGP = 2,944 EUR meets it, the conditions left.
		_, b = do(srv, http.MethodGet, "/tools/visa/result?country=ES&visa=digital-nomad&income=160,000", nil)
		if !strings.Contains(b, "Tick the conditions") {
			t.Fatalf("typed income: %s", b)
		}
		_, b = do(srv, http.MethodGet, "/tools/visa/result?country=ES&visa=digital-nomad&income=160000&check_1=1&check_2=1&check_3=1&check_4=1", nil)
		if !strings.Contains(b, "Ready") {
			t.Errorf("all ticked: %s", b)
		}
		// A dependent: +75% of the SMI.
		_, b = do(srv, http.MethodGet, "/tools/visa/result?country=ES&visa=non-lucrative&dependents=1", nil)
		if !strings.Contains(b, "36,000") || !strings.Contains(b, "One of these") {
			t.Errorf("NLV with a dependent: %s", b)
		}
	})

	t.Run("months and funds", func(t *testing.T) {
		srv := newServer(nil, 5)
		_, b := do(srv, http.MethodGet, "/tools/visa/result?country=PT&visa=d8", nil)
		// 0 of the last 3 months reached 3,680; 11,040 − 9,200 to save, in
		// EGP as EUR isn't one of the user's shown currencies.
		if !strings.Contains(b, "Last 3 months: 0 reached") {
			t.Errorf("no months proof: %s", b)
		}
		if !strings.Contains(b, "Save for it") || !strings.Contains(b, "amount=100000") || !strings.Contains(b, "currency_id="+egp.ID) {
			t.Fatalf("no goal for the funds: %s", b)
		}
		res, b := do(srv, http.MethodGet, "/decide/afford?name=D8+funds&amount=1840&currency_id="+egp.ID, nil)
		if res.StatusCode != http.StatusOK || !strings.Contains(b, `value="D8 funds"`) || !strings.Contains(b, "1840") {
			t.Errorf("plan it not prefilled: %s", b)
		}
	})

	t.Run("options and offer", func(t *testing.T) {
		srv := newServer(nil, 5)
		_, b := do(srv, http.MethodGet, "/tools/visa/result?country=DE&visa=blue-card", nil)
		if !strings.Contains(b, "Needs a job offer") {
			t.Errorf("no offer: %s", b)
		}
		_, b = do(srv, http.MethodGet, "/tools/visa/result?country=DE&visa=blue-card&opt_job=reduced&offer=46000&check_1=1&check_2=1", nil)
		if !strings.Contains(b, "45,934") || !strings.Contains(b, "Ready") || !strings.Contains(b, "/tools/salary?") {
			t.Errorf("reduced blue card: %s", b)
		}
	})

	t.Run("conditions only", func(t *testing.T) {
		srv := newServer(nil, 5)
		_, b := do(srv, http.MethodGet, "/tools/visa/result?country=US&visa=h-1b", nil)
		if !strings.Contains(b, "No amount of money is asked") || !strings.Contains(b, "Tick the conditions") || !strings.Contains(b, "prevailing wage") {
			t.Errorf("H-1B: %s", b)
		}
	})

	t.Run("ai", func(t *testing.T) {
		g := &fakeVisaGuesser{}
		srv := newServer(g, 2)
		_, b := do(srv, http.MethodGet, "/tools/visa/result?country=GR&visa=ai&name=golden", nil)
		if !strings.Contains(b, "Look up the visa") || !strings.Contains(b, "2 left this month") {
			t.Fatalf("no lookup offered: %s", b)
		}
		form := url.Values{"country": {"GR"}, "visa": {"ai"}, "name": {"golden"}}
		res, b := do(srv, http.MethodPost, "/tools/visa/rules/fetch", form)
		if res.StatusCode != http.StatusOK || !strings.Contains(b, "AI lookup") || !strings.Contains(b, "250,000") || g.calls != 1 {
			t.Fatalf("status = %d, calls = %d, body = %s", res.StatusCode, g.calls, b)
		}
		if g.lastIn.Name != "Greece" || g.lastIn.Currency != "EUR" || g.lastIn.Visa != "golden" {
			t.Errorf("asked %+v", g.lastIn)
		}
		// Kept under the typed name and the official one: no call.
		_, b = do(srv, http.MethodGet, "/tools/visa/result?country=GR&visa=ai&name=Golden+Visa", nil)
		if g.calls != 1 || visaScans() != 1 || !strings.Contains(b, "250,000") {
			t.Errorf("calls = %d, scans = %d", g.calls, visaScans())
		}
		form.Set("refresh", "1")
		do(srv, http.MethodPost, "/tools/visa/rules/fetch", form)
		_, b = do(srv, http.MethodPost, "/tools/visa/rules/fetch", form)
		if g.calls != 2 || visaScans() != 2 || !strings.Contains(b, "the most you can") {
			t.Errorf("calls = %d, scans = %d, body = %s", g.calls, visaScans(), b)
		}
	})

	t.Run("own rules", func(t *testing.T) {
		srv := newServer(nil, 5)
		bad := url.Values{"name": {"Mine"}, "country": {"TH"}, "currency": {"THB"}, "req_kind": {"income"}, "req_period": {"month"}, "req_amount": {""}}
		if res, b := do(srv, http.MethodPost, "/tools/visa/rules", bad); res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(b, "over 0") {
			t.Fatalf("status = %d, body = %s", res.StatusCode, b)
		}
		form := url.Values{
			"name": {"Thai DTV"}, "country": {"TH"}, "currency": {"THB"}, "category": {"nomad"},
			"req_kind": {"savings", "check"}, "req_period": {"once", ""}, "req_amount": {"500000", ""},
			"req_per_dep": {"", ""}, "req_group": {"", ""}, "req_months": {"", ""}, "req_text": {"", "Remote work"},
		}
		res, _ := do(srv, http.MethodPost, "/tools/visa/rules", form)
		var trig struct {
			Changed struct{ ID string } `json:"visa-rules-changed"`
		}
		if err := json.Unmarshal([]byte(res.Header.Get("HX-Trigger")), &trig); err != nil || trig.Changed.ID == "" {
			t.Fatalf("status = %d, trigger = %q", res.StatusCode, res.Header.Get("HX-Trigger"))
		}
		id := trig.Changed.ID
		// Its id picks its country.
		_, b := do(srv, http.MethodGet, "/tools/visa/result?visa="+id, nil)
		if !strings.Contains(b, "Thai DTV") || !strings.Contains(b, "Yours") || !strings.Contains(b, "Remote work") {
			t.Errorf("own rules: %s", b)
		}
		// Copied from a built in one.
		_, b = do(srv, http.MethodGet, "/tools/visa/rules/new?from=1&country=PT&visa=d7", nil)
		if !strings.Contains(b, "D7 Passive Income Visa (my copy)") {
			t.Errorf("copy form: %s", b)
		}
		if res, _ := do(srv, http.MethodDelete, "/tools/visa/rules/"+id, nil); res.StatusCode != http.StatusOK {
			t.Errorf("delete status = %d", res.StatusCode)
		}
		_, b = do(srv, http.MethodGet, "/tools/visa/result?visa="+id, nil)
		if strings.Contains(b, "Thai DTV") {
			t.Error("deleted rules still used")
		}
	})

	t.Run("compare", func(t *testing.T) {
		srv := newServer(nil, 5)
		res, page := do(srv, http.MethodGet, "/tools/visa/compare", nil)
		if res.StatusCode != http.StatusOK || !strings.Contains(page, "Visa 2") || !strings.Contains(page, "Closest") {
			t.Fatalf("status = %d, page = %s", res.StatusCode, page)
		}
		// Canada's funds (CAD 15,263 ≈ 545,107 EGP) against Spain's nomad.
		res, b := do(srv, http.MethodGet, "/tools/visa/compare/result?n=2&v0_country=CA&v0_visa=express-entry&v1_country=ES&v1_visa=digital-nomad&savings=600000", nil)
		if !strings.Contains(b, "15,263") {
			t.Errorf("no canadian funds: %s", b)
		}
		if i, j := strings.Index(b, "Closest"), strings.Index(b, "Visa 2"); i < 0 || i > j {
			t.Errorf("the closest isn't the first visa")
		}
		if u := res.Header.Get("HX-Replace-Url"); !strings.Contains(u, "v0_country=CA") || !strings.Contains(u, "savings=600000") {
			t.Errorf("replace url = %q", u)
		}
		_, b = do(srv, http.MethodGet, "/tools/visa/compare/result?n=3&remove=0&v0_country=CA&v1_country=ES&v2_country=PT", nil)
		if strings.Contains(b, "Visa 3") {
			t.Error("visa not removed")
		}
	})
}
