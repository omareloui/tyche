package router_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
	"github.com/omareloui/tyche/web/views"
)

func TestSettings(t *testing.T) {
	repo, ctx, usr := setupTenant(t)

	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	usd, err := repo.GetCurrencyByString(ctx, "USD")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.FirstName, usr.LastName, usr.DefaultCurrency = "e2e@example.com", "End", "ToEnd", *egp
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	pair, err := jwt.NewPair(usr)
	if err != nil {
		t.Fatal(err)
	}
	access := &http.Cookie{Name: "access_token", Value: pair.Access.Encoded}

	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)))
	t.Cleanup(srv.Close)

	do := func(method, path string, form url.Values, htmx bool) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if htmx {
			req.Header.Set("HX-Request", "true")
		}
		req.AddCookie(access)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}
	newAccess := func(res *http.Response) *jwt.AccessClaims {
		t.Helper()
		for _, c := range res.Cookies() {
			if c.Name == "access_token" {
				claims, err := jwt.ParseAccessClaims(c.Value)
				if err != nil {
					t.Fatal(err)
				}
				return claims
			}
		}
		t.Fatal("no access cookie reissued")
		return nil
	}
	user := func() *models.User {
		t.Helper()
		u, err := repo.GetUserByOAuthID(ctx, usr.OAuthID)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}

	res, body := do("GET", "/settings", nil, false)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "Start of the month") || !strings.Contains(body, "Salary and work hours") {
		t.Fatalf("GET /settings = %d", res.StatusCode)
	}
	// The page's links scroll to its cards.
	for _, card := range []string{views.SettingsProfile, views.SettingsRegion, views.SettingsMonth, views.SettingsWork, views.SettingsAccount} {
		id := "settings-" + card
		if !strings.Contains(body, `href="#`+id+`"`) || !strings.Contains(body, `id="`+id+`"`) {
			t.Errorf("no link to #%s, or no card with that id", id)
		}
	}

	t.Run("htmx only", func(t *testing.T) {
		if res, _ := do("PUT", "/settings/month", url.Values{"month_start": {"25"}}, false); res.StatusCode != http.StatusForbidden {
			t.Errorf("status = %d, want 403", res.StatusCode)
		}
	})

	t.Run("month start", func(t *testing.T) {
		for _, bad := range []string{"0", "32", "x"} {
			_, body := do("PUT", "/settings/month", url.Values{"month_start": {bad}}, true)
			if !strings.Contains(body, "Pick a day from 1 to 31") {
				t.Errorf("month_start=%s: no error", bad)
			}
		}
		if user().StartOfPeriod != 1 {
			t.Fatal("saved an invalid day")
		}

		res, body := do("PUT", "/settings/month", url.Values{"month_start": {"25"}}, true)
		if !strings.Contains(body, "Saved") {
			t.Errorf("not saved: %s", body)
		}
		if got := user().StartOfPeriod; got != 25 {
			t.Errorf("start_of_period = %d", got)
		}
		if c := newAccess(res); c.MonthStart != 25 {
			t.Errorf("claims month start = %d", c.MonthStart)
		}
	})

	t.Run("profile", func(t *testing.T) {
		_, body := do("PUT", "/settings/profile", url.Values{"first_name": {" "}, "last_name": {"X"}}, true)
		if !strings.Contains(body, "Enter a name") {
			t.Error("empty first name accepted")
		}
		res, _ := do("PUT", "/settings/profile", url.Values{"first_name": {" Omar "}, "last_name": {"E"}}, true)
		if u := user(); u.FirstName != "Omar" || u.LastName != "E" {
			t.Errorf("name = %q %q", u.FirstName, u.LastName)
		}
		if c := newAccess(res); c.Name != "Omar E" {
			t.Errorf("claims name = %q", c.Name)
		}
	})

	t.Run("work", func(t *testing.T) {
		_, body := do("PUT", "/settings/work", url.Values{"amount": {"0"}, "currency_id": {egp.ID}, "work_hours_per_week": {"40"}}, true)
		if !strings.Contains(body, "Enter your monthly salary") {
			t.Error("no salary accepted")
		}
		do("PUT", "/settings/work", url.Values{"amount": {"20,000"}, "currency_id": {egp.ID}, "work_hours_per_week": {"35"}}, true)
		ws, err := repo.GetWorkSettings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if ws.MonthlySalary != 20000 || ws.HoursPerWeek != 35 || ws.Currency.ID != egp.ID {
			t.Errorf("work settings = %+v", ws)
		}
	})

	t.Run("region", func(t *testing.T) {
		_, body := do("PUT", "/settings/region", url.Values{"currency_id": {egp.ID}, "timezone": {"Mars/Base"}}, true)
		if !strings.Contains(body, "Unknown time zone") {
			t.Error("bad time zone accepted")
		}
		res, _ := do("PUT", "/settings/region", url.Values{"currency_id": {egp.ID}, "timezone": {"Asia/Dubai"}}, true)
		if res.Header.Get("HX-Refresh") != "" {
			t.Error("refreshed without a currency change")
		}
		if loc, _ := repo.GetTimezone(ctx); loc.String() != "Asia/Dubai" {
			t.Errorf("timezone = %s", loc)
		}

		res, _ = do("PUT", "/settings/region", url.Values{"currency_id": {usd.ID}, "timezone": {"Asia/Dubai"}}, true)
		if res.Header.Get("HX-Refresh") != "true" {
			t.Error("no refresh on a currency change")
		}
		if c := newAccess(res); c.DefaultCurrency.ID != usd.ID {
			t.Errorf("claims currency = %s", c.DefaultCurrency.String)
		}
		// Unlike the onboarding's, no account moves to the new currency.
		accs, err := repo.GetAccounts(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range accs {
			if a.ID == bank.ID && a.Currency.ID != egp.ID {
				t.Errorf("bank moved to %s", a.Currency.String)
			}
		}
	})

	t.Run("default account", func(t *testing.T) {
		_, body := do("PUT", "/settings/account", url.Values{"account_id": {"nope"}}, true)
		if !strings.Contains(body, "Pick an account") {
			t.Error("unknown account accepted")
		}
		res, _ := do("PUT", "/settings/account", url.Values{"account_id": {bank.ID}}, true)
		if u := user(); u.DefaultAccount == nil || u.DefaultAccount.ID != bank.ID {
			t.Errorf("default account = %+v", u.DefaultAccount)
		}
		if c := newAccess(res); c.DefaultAccount == nil || c.DefaultAccount.ID != bank.ID {
			t.Errorf("claims default account = %+v", c.DefaultAccount)
		}
	})
}

// TestMonthStartAnalytics checks the analytics count the user's months, here
// from the 25th: March is Feb 25 – Mar 24.
func TestMonthStartAnalytics(t *testing.T) {
	repo, ctx, usr := setupTenant(t)
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)

	for _, tx := range []struct {
		day    string
		amount float64
	}{
		{"2026-02-24", 1},   // February
		{"2026-02-25", 10},  // March
		{"2026-03-24", 100}, // March
		{"2026-03-25", 1000},
	} {
		d, _ := time.Parse(time.DateOnly, tx.day)
		at := d.Add(12 * time.Hour)
		if err := repo.CreateTransaction(ctx, &models.Transaction{
			ID: idgen.New(), Account: *bank, Kind: models.ExpenseTxKind, Amount: tx.amount,
			ScheduledTo: at, FulfilledAt: at,
		}, nil, nil); err != nil {
			t.Fatal(err)
		}
	}

	ctx25 := context.WithValue(ctx, middleware.AccessClaimsCtxKey{}, &jwt.AccessClaims{
		ID: usr.ID, OAuthID: usr.OAuthID, OAuthProvider: usr.OAuthProvider, MonthStart: 25,
	})
	march := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	egp := bank.Currency

	bs, err := repo.GetBudgetSummary(ctx25, march, &egp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bs.Period.RangeLabel() != "Feb 25 – Mar 24" || bs.Spent() != 110 || bs.Prev != "2026-02" || bs.Next != "2026-04" {
		t.Errorf("budget summary = %s, spent %v, prev %q, next %q", bs.Period.RangeLabel(), bs.Spent(), bs.Prev, bs.Next)
	}

	cs, err := repo.GetCategorySpending(ctx25, march, &egp)
	if err != nil {
		t.Fatal(err)
	}
	if cs.Total != 110 {
		t.Errorf("category spending = %v", cs.Total)
	}

	until := time.Time{}
	page, err := repo.GetTransactionsPage(ctx25, models.TxFilter{}, time.Time{}, &until)
	if err != nil {
		t.Fatal(err)
	}
	if page.MonthStart != 25 {
		t.Errorf("page month start = %d", page.MonthStart)
	}
	months := map[string]float64{}
	for _, tx := range page.History {
		months[page.MonthStart.Of(tx.ScheduledTo).Key()] += tx.Amount
	}
	if months["2026-02"] != 1 || months["2026-03"] != 110 || months["2026-04"] != 1000 {
		t.Errorf("months = %v", months)
	}

	// The list's month headers split on the 25th too.
	var b strings.Builder
	if err := views.TxListChunk(views.TxListData{Page: page, Def: &egp}).Render(ctx25, &b); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"(Mar 25 – Apr 24)", "(Feb 25 – Mar 24)", "(Jan 25 – Feb 24)"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("list has no %s header", want)
		}
	}
}
