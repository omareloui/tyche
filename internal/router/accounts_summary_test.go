package router_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
	"github.com/omareloui/tyche/web/views"
)

func TestAccountsSummary(t *testing.T) {
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

	kinds, err := repo.GetAccountKinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kind := map[models.AccKind]models.AccountKind{}
	for _, k := range kinds {
		kind[k.Enum] = k
	}
	// Drop the seeded accounts, so only these count.
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
	bank := &models.Account{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZA", Name: "Bank", Amount: 1000, Currency: *egp, Kind: kind[models.AccKindBankAcc]}
	card := &models.Account{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZB", Name: "Visa", Amount: -250, CardLimit: 1000, Currency: *egp, Kind: kind[models.AccKindCreditCard]}
	hidden := &models.Account{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZC", Name: "Hidden", Amount: 99999, ExcludeFromBalance: true, Currency: *egp, Kind: kind[models.AccKindBankAcc]}
	for _, a := range []*models.Account{bank, card, hidden} {
		if err := repo.CreateAccount(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	tx := &models.Transaction{
		ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZD", Account: *bank, Amount: 120, Kind: models.ExpenseTxKind,
		ScheduledTo: time.Now(), FulfilledAt: time.Now(),
	}
	if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)))
	t.Cleanup(srv.Close)

	do := func(method, path string, form url.Values) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}

	t.Run("the page loads it", func(t *testing.T) {
		_, body := do("GET", "/accounts", nil)
		if !strings.Contains(body, `hx-get="/accounts/summary"`) {
			t.Error("page doesn't load the summary")
		}
	})

	t.Run("net worth, kinds and cards", func(t *testing.T) {
		res, body := do("GET", "/accounts/summary", nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		// Bank 1000 − 120 = 880, card −250: 630.
		for _, want := range []string{egp.Format(630), egp.Format(880) + "</span> assets", egp.Format(250) + "</span> owed", "2 accounts", "1 card", "25% of the limits used", "Over time", "By kind"} {
			if !strings.Contains(body, want) {
				t.Errorf("summary misses %q", want)
			}
		}
		if strings.Contains(body, "99,999") || strings.Contains(body, "By currency") {
			t.Error("summary counts the excluded account or shows a single currency")
		}
	})

	t.Run("including the excluded accounts", func(t *testing.T) {
		_, body := do("GET", "/accounts/summary", nil)
		if !strings.Contains(body, `data-acc-summary-with="excluded" aria-pressed="false"`) {
			t.Error("no toggle to include the excluded account")
		}
		_, body = do("GET", "/accounts/summary?with=excluded", nil)
		for _, want := range []string{egp.Format(100629), "3 accounts", `data-acc-summary-with="excluded" aria-pressed="true"`} {
			if !strings.Contains(body, want) {
				t.Errorf("summary misses %q", want)
			}
		}
	})

	t.Run("less the budget left", func(t *testing.T) {
		cats, err := repo.GetCategories(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var food models.Category
		for _, c := range cats {
			if c.HasKind(models.ExpenseTxKind) {
				food = c
				break
			}
		}
		food.BudgetAmount, food.BudgetCurrency = 200, egp
		if err := repo.UpdateCategory(ctx, &food); err != nil {
			t.Fatal(err)
		}
		_, body := do("GET", "/accounts/summary", nil)
		// Forecast either way, its parts told.
		for _, want := range []string{"dashed: forecast", "How the forecast works", "This month’s budgets", egp.Format(200)} {
			if !strings.Contains(body, want) {
				t.Errorf("summary without the budgets misses %q", want)
			}
		}
		_, body = do("GET", "/accounts/summary?with=budget", nil)
		// 630 − 200 left (the expense is uncategorized).
		for _, want := range []string{egp.Format(430), egp.Format(200) + "</span> left in this month", `data-acc-summary-with="budget" aria-pressed="true"`, "dashed: forecast"} {
			if !strings.Contains(body, want) {
				t.Errorf("summary misses %q", want)
			}
		}
	})

	t.Run("account changes re-render it", func(t *testing.T) {
		res, body := do("PUT", "/accounts/"+card.ID+"/archive", nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		if !strings.Contains(res.Header.Get("HX-Trigger"), views.AccountsChangedEvent) {
			t.Errorf("HX-Trigger = %q", res.Header.Get("HX-Trigger"))
		}
	})
}
