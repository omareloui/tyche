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

func TestCounterpartiesSummary(t *testing.T) {
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
	var bankKind models.AccountKind
	for _, k := range kinds {
		if k.Enum == models.AccKindBankAcc {
			bankKind = k
		}
	}
	bank := &models.Account{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZA", Name: "Bank", Amount: 1000, Currency: *egp, Kind: bankKind}
	if err := repo.CreateAccount(ctx, bank); err != nil {
		t.Fatal(err)
	}
	cafe := &models.Counterparty{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZC", Name: "Cafe", IsMerchant: true, Labels: []string{"Coffee shop"}}
	friend := &models.Counterparty{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZE", Name: "Friend"}
	for _, cp := range []*models.Counterparty{cafe, friend} {
		if err := repo.CreateCounterparty(ctx, cp); err != nil {
			t.Fatal(err)
		}
	}
	for i, cp := range []*models.Counterparty{cafe, friend} {
		tx := &models.Transaction{
			ID: []string{"01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZD", "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZF"}[i], Account: *bank, Amount: 120, Kind: models.ExpenseTxKind,
			Counterparty: cp, ScheduledTo: time.Now(), FulfilledAt: time.Now(),
		}
		if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
			t.Fatal(err)
		}
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
		_, body := do("GET", "/counterparties", nil)
		if !strings.Contains(body, `hx-get="/counterparties/summary"`) {
			t.Error("page doesn't load the summary")
		}
	})

	t.Run("merchants and labels", func(t *testing.T) {
		res, body := do("GET", "/counterparties/summary", nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		for _, want := range []string{"Cafe", `data-label="Coffee shop"`, "at 1 merchant", `data-cp-summary-toggle="` + cafe.ID + `"`} {
			if !strings.Contains(body, want) {
				t.Errorf("summary misses %q", want)
			}
		}
		// Not a merchant.
		if strings.Contains(body, "Friend") {
			t.Error("summary shows a non-merchant")
		}
	})

	t.Run("hidden merchants", func(t *testing.T) {
		_, body := do("GET", "/counterparties/summary?hide="+cafe.ID, nil)
		if !strings.Contains(body, "Hidden:") || !strings.Contains(body, "at 0 merchants") {
			t.Errorf("cafe not hidden: %s", body)
		}
	})

	t.Run("a past month links forward", func(t *testing.T) {
		prev := time.Now().AddDate(0, -1, 0).Format("2006-01")
		_, body := do("GET", "/counterparties/summary?month="+prev, nil)
		if !strings.Contains(body, `data-cp-summary-month="`+time.Now().Format("2006-01")+`"`) {
			t.Error("no next month link")
		}
	})

	t.Run("an invalid month", func(t *testing.T) {
		if res, _ := do("GET", "/counterparties/summary?month=nope", nil); res.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("status %d, want 422", res.StatusCode)
		}
	})

	t.Run("label changes re-render it", func(t *testing.T) {
		res, body := do("PUT", "/counterparties/"+cafe.ID+"/labels", url.Values{"labels": {"Coffee shop", "Breakfast"}})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		if !strings.Contains(res.Header.Get("HX-Trigger"), views.CounterpartiesChangedEvent) {
			t.Errorf("HX-Trigger = %q", res.Header.Get("HX-Trigger"))
		}
	})
}
