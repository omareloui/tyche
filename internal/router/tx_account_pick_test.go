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
)

// TestTxAccountPick moves a transaction to another account of its currency
// from its details, its balance effect moving with it.
func TestTxAccountPick(t *testing.T) {
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
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	cash := newTestAccount(t, repo, ctx, "Wallet", "EGP", models.AccKindCash, 500)
	usd := newTestAccount(t, repo, ctx, "Dollars", "USD", models.AccKindBankAcc, 50)

	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)))
	t.Cleanup(srv.Close)
	do := func(method, path string, form url.Values) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Target", "txd-panel")
		req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}
	balance := func(id string) float64 {
		t.Helper()
		a, err := repo.GetAccount(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return a.Amount
	}

	res, body := do("POST", "/transactions", url.Values{
		"kind": {models.ExpenseTxKind.String()}, "account_id": {bank.ID}, "amount": {"100"},
		"scheduled_to": {time.Now().Format(time.DateOnly)}, "description": {"Coffee"}, "is_recurrent": {""},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", res.StatusCode, body)
	}
	page, err := repo.GetTransactionsPage(ctx, models.TxFilter{}, time.Time{}, &time.Time{})
	if err != nil || len(page.History) != 1 {
		t.Fatalf("history = %v, %v", len(page.History), err)
	}
	tx := page.History[0]
	path := "/transactions/" + tx.ID + "/pick/account"

	_, body = do("GET", path, nil)
	if !strings.Contains(body, cash.ID) || strings.Contains(body, usd.ID) {
		t.Error("picker should offer the EGP accounts only")
	}

	if res, body = do("PUT", path, url.Values{"account_id": {usd.ID}}); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("another currency: status %d", res.StatusCode)
	}

	if res, body = do("PUT", path, url.Values{"account_id": {cash.ID}}); res.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", res.StatusCode, body)
	}
	if b, c := balance(bank.ID), balance(cash.ID); b != 1000 || c != 400 {
		t.Errorf("bank = %v, cash = %v, want 1000 / 400", b, c)
	}
	got, err := repo.GetTransaction(ctx, tx.ID)
	if err != nil || got.Account.ID != cash.ID {
		t.Fatalf("account = %v, %v", got.Account.ID, err)
	}
	if !strings.Contains(body, "Wallet") {
		t.Error("details not answered")
	}
}
