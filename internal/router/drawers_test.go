package router_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
	"github.com/omareloui/tyche/web/views"
)

// TestFormDrawers goes through a create and an edit form in their drawers,
// with categories: the edit form opens as a drawer, an invalid one answers
// with the form alone, a saved one with its card retargeted and closes the
// drawer; a created one closes its drawer once it slid out.
func TestFormDrawers(t *testing.T) {
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
	food := &models.Category{
		ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZB", Name: "Food e2e", Color: "#000000",
		Kinds: []models.TxKind{models.ExpenseTxKind},
	}
	if err := repo.CreateCategory(ctx, food); err != nil {
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
	triggers := func(res *http.Response) string { return res.Header.Get("HX-Trigger") }

	t.Run("the edit form opens in a drawer", func(t *testing.T) {
		res, body := do("GET", "/categories/"+food.ID+"/edit", nil)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "data-drawer") || !strings.Contains(body, "data-drawer-form") {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
	})

	t.Run("an invalid edit answers with the form alone", func(t *testing.T) {
		res, body := do("PUT", "/categories/"+food.ID, url.Values{"name": {""}, "color": {"#000000"}, "kind": {"Expense"}, "currency_id": {egp.ID}})
		if strings.Contains(triggers(res), "drawer-saved") || strings.Contains(body, "drawer-root") || !strings.Contains(body, "data-drawer-form") {
			t.Fatalf("status %d, trigger %q: %s", res.StatusCode, triggers(res), body)
		}
	})

	t.Run("a saved edit retargets to the card and closes the drawer", func(t *testing.T) {
		res, body := do("PUT", "/categories/"+food.ID, url.Values{"name": {"Snacks e2e"}, "color": {"#000000"}, "kind": {"Expense"}, "currency_id": {egp.ID}})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		if got, want := res.Header.Get("HX-Retarget"), "#"+views.CategoryID(food.ID); got != want {
			t.Errorf("HX-Retarget = %q, want %q", got, want)
		}
		if tr := triggers(res); !strings.Contains(tr, "drawer-saved") || !strings.Contains(tr, views.BudgetChangedEvent) {
			t.Errorf("HX-Trigger = %q, want drawer-saved and %s", tr, views.BudgetChangedEvent)
		}
		if !strings.Contains(body, "Snacks e2e") || !strings.Contains(body, `id="`+views.CategoryID(food.ID)+`"`) {
			t.Errorf("body isn't the updated card: %s", body)
		}
	})

	t.Run("a created one closes its drawer, swapping the fresh form in after", func(t *testing.T) {
		res, body := do("POST", "/categories", url.Values{"name": {"Rent e2e"}, "color": {"#000000"}, "kind": {"Expense"}, "currency_id": {egp.ID}})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		if !strings.Contains(triggers(res), "drawer-saved") || res.Header.Get("HX-Reswap") != "outerHTML swap:200ms" {
			t.Errorf("trigger %q, reswap %q", triggers(res), res.Header.Get("HX-Reswap"))
		}
		if !strings.Contains(body, "Rent e2e") || !strings.Contains(body, "data-drawer-static") {
			t.Errorf("body lacks the new category or the fresh form: %s", body)
		}
	})
}
