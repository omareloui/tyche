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

func TestTransactionDetails(t *testing.T) {
	repo, ctx, _ := setupTenant(t)

	egp, err := repo.GetCurrencyByString(ctx, "EGP")
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
	food := &models.Category{
		ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZB", Name: "Food", Color: "#000000",
		BudgetAmount: 500, BudgetCurrency: egp, Kinds: []models.TxKind{models.ExpenseTxKind},
	}
	if err := repo.CreateCategory(ctx, food); err != nil {
		t.Fatal(err)
	}
	cafe := &models.Counterparty{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZC", Name: "Cafe", IsMerchant: true}
	if err := repo.CreateCounterparty(ctx, cafe); err != nil {
		t.Fatal(err)
	}

	month := models.MonthOf(time.Now())
	txs := []*models.Transaction{
		{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZD", Amount: 100, Description: "Lunch"},
		{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZE", Amount: 50, Description: "Coffee"},
		// Last month's, out of the month's stats but still related.
		{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZF", Amount: 30, Description: "Cake", ScheduledTo: month.AddDate(0, 0, -3)},
	}
	for _, tx := range txs {
		tx.Account, tx.Kind, tx.Category, tx.Counterparty = *bank, models.ExpenseTxKind, food, cafe
		if tx.ScheduledTo.IsZero() {
			tx.ScheduledTo = month
		}
		tx.FulfilledAt = tx.ScheduledTo
		if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
			t.Fatal(err)
		}
	}

	d, err := repo.GetTransactionDetails(ctx, txs[0].ID, egp)
	if err != nil {
		t.Fatal(err)
	}

	if d.Tx.ID != txs[0].ID {
		t.Errorf("tx = %s, want %s", d.Tx.ID, txs[0].ID)
	}
	var related []string
	for _, rt := range d.CounterpartyTxs {
		related = append(related, rt.Description)
	}
	if strings.Join(related, ",") != "Coffee,Cake" {
		t.Errorf("counterparty txs = %v, want [Coffee Cake]", related)
	}
	if s := d.CounterpartyMonth; s == nil || s.Count != 2 || s.Out != 150 {
		t.Errorf("counterparty month = %+v, want 2 txs, 150 out", s)
	}
	if b := d.Budget; b == nil || b.Spent != 150 || b.Budget != 500 {
		t.Errorf("budget = %+v, want 150 of 500", b)
	}
	if d.SMS != nil || d.Recurrent != nil || d.Pair != nil || d.Reimburses != nil {
		t.Errorf("got links it doesn't have: %+v", d)
	}

	var sb strings.Builder
	if err := views.TxDetails(d, egp).Render(ctx, &sb); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`data-tx-details`, "Latest with Cafe", "Coffee", "Food"} {
		if !strings.Contains(sb.String(), want) {
			t.Errorf("details don't show %q", want)
		}
	}
}

func TestEditTransactionInDetails(t *testing.T) {
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
	tx := &models.Transaction{
		ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZD", Account: *bank, Amount: 100, Kind: models.ExpenseTxKind,
		Description: "Lunch", ScheduledTo: time.Now(), FulfilledAt: time.Now(),
	}
	if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)))
	t.Cleanup(srv.Close)

	do := func(method, path, target string, form url.Values) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		if target != "" {
			req.Header.Set("HX-Target", target)
		}
		req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}
	editURL := "/transactions/" + tx.ID + "/edit"

	t.Run("from a row, the drawer opens in the form", func(t *testing.T) {
		res, body := do("GET", editURL, "tx-details", nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		for _, want := range []string{"data-tx-details", `value="menu"`, `hx-target="#txd-panel"`} {
			if !strings.Contains(body, want) {
				t.Errorf("missing %q", want)
			}
		}
	})

	t.Run("from the details, only the form is swapped in", func(t *testing.T) {
		res, body := do("GET", editURL, "txd-panel", nil)
		if res.StatusCode != http.StatusOK || strings.Contains(body, "data-tx-details") || !strings.Contains(body, `value="details"`) {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
	})

	form := url.Values{
		"from": {"details"}, "account_id": {bank.ID}, "kind": {"Expense"},
		"amount": {"120"}, "description": {"Dinner"}, "scheduled_to": {time.Now().Format("2006-01-02")},
	}

	t.Run("saving answers the details and the list", func(t *testing.T) {
		res, body := do("PUT", "/transactions/"+tx.ID, "txd-panel", form)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		if got := res.Header.Get("HX-Trigger"); got != "drawer-saved" {
			t.Errorf("HX-Trigger = %q, want drawer-saved", got)
		}
		for _, want := range []string{`hx-swap-oob`, "Dinner", "drawer-panel flex"} {
			if !strings.Contains(body, want) {
				t.Errorf("missing %q", want)
			}
		}
		saved, err := repo.GetTransaction(ctx, tx.ID)
		if err != nil {
			t.Fatal(err)
		}
		if saved.Amount != 120 || saved.Description != "Dinner" {
			t.Errorf("saved %v %q, want 120 Dinner", saved.Amount, saved.Description)
		}
	})

	t.Run("an invalid form stays open with its errors", func(t *testing.T) {
		bad := url.Values{}
		for k, v := range form {
			bad[k] = v
		}
		bad.Set("amount", "")
		bad.Set("from", "menu")
		res, body := do("PUT", "/transactions/"+tx.ID, "txd-panel", bad)
		if res.Header.Get("HX-Trigger") == "drawer-saved" || !strings.Contains(body, `value="menu"`) {
			t.Fatalf("status %d, trigger %q: %s", res.StatusCode, res.Header.Get("HX-Trigger"), body)
		}
	})
}
