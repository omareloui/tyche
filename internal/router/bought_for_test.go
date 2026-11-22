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

// TestBoughtForMe saves an expense someone bought for the user with the debt
// borrowed from them, then links, unlinks and edits them.
func TestBoughtForMe(t *testing.T) {
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
	sara := &models.Counterparty{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8C1", Name: "Sara"}
	if err := repo.CreateCounterparty(ctx, sara); err != nil {
		t.Fatal(err)
	}
	saraAcc := newTestAccount(t, repo, ctx, "Sara", "EGP", models.AccKindDebt, 0)
	saraAcc.Counterparty = sara

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
	balance := func(id string) float64 {
		t.Helper()
		a, err := repo.GetAccount(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return a.Amount
	}
	today := time.Now().Format(time.DateOnly)
	create := func(desc, amount string, extra url.Values) (*http.Response, string) {
		t.Helper()
		form := url.Values{
			"kind": {models.ExpenseTxKind.String()}, "account_id": {bank.ID}, "amount": {amount},
			"scheduled_to": {today}, "description": {desc}, "is_recurrent": {""},
		}
		for k, v := range extra {
			form[k] = v
		}
		return do("POST", "/transactions", form)
	}
	find := func(desc string, kind models.TxKind) *models.Transaction {
		t.Helper()
		txs, err := repo.GetTransactionsPage(ctx, models.TxFilter{}, time.Time{}, &time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		for _, list := range [][]models.Transaction{txs.Upcoming, txs.History} {
			for i := range list {
				if list[i].Description == desc && list[i].Kind == kind {
					return &list[i]
				}
				for j := range list[i].BoughtBy {
					if d := &list[i].BoughtBy[j]; d.Description == desc && d.Kind == kind {
						return d
					}
				}
			}
		}
		return nil
	}

	var expense *models.Transaction
	t.Run("the form saves the expense and the debt, linked", func(t *testing.T) {
		res, body := create("Headphones", "300", url.Values{"bought_by_on": {"on"}, "bought_by": {saraAcc.ID}})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		expense = find("Headphones", models.ExpenseTxKind)
		if expense == nil {
			t.Fatal("no expense")
		}
		if len(expense.BoughtBy) != 1 {
			t.Fatalf("bought by = %d debts", len(expense.BoughtBy))
		}
		d := expense.BoughtBy[0]
		if d.Account.ID != saraAcc.ID || d.ToAccount.ID != bank.ID || d.Amount != 300 || d.ToAmount != 300 || d.BoughtForID != expense.ID {
			t.Errorf("debt = %+v", d)
		}
		if b := balance(bank.ID); b != 1000 {
			t.Errorf("bank = %v, want 1000 (in and out)", b)
		}
		if s := balance(saraAcc.ID); s != -300 {
			t.Errorf("sara = %v, want -300", s)
		}
		// The debt is nested, not a row of its own.
		if find("Headphones", models.DebtTxKind) == nil {
			t.Error("debt not nested under the expense")
		}
		page, _ := repo.GetTransactionsPage(ctx, models.TxFilter{}, time.Time{}, &time.Time{})
		for _, tx := range page.History {
			if tx.Kind == models.DebtTxKind {
				t.Errorf("linked debt listed on its own: %+v", tx.ID)
			}
		}
		// Filtering by her account still shows the debt.
		page, _ = repo.GetTransactionsPage(ctx, models.TxFilter{AccountID: saraAcc.ID}, time.Time{}, &time.Time{})
		if len(page.History) != 1 || page.History[0].Kind != models.DebtTxKind {
			t.Errorf("her account's list = %+v", page.History)
		}
	})

	t.Run("a person is required", func(t *testing.T) {
		res, body := create("Nobody", "50", url.Values{"bought_by_on": {"on"}})
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Pick who bought it") {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		if find("Nobody", models.ExpenseTxKind) != nil {
			t.Error("saved without a person")
		}
	})

	t.Run("a new person gets a debt account", func(t *testing.T) {
		res, body := do("POST", "/transactions/bought-by/person", url.Values{"name": {"Omar K"}})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		accs, _ := repo.GetAccounts(ctx)
		var acc *models.Account
		for i := range accs {
			if accs[i].Name == "Omar K" && accs[i].Kind.Enum == models.AccKindDebt {
				acc = &accs[i]
			}
		}
		if acc == nil || acc.Counterparty == nil || acc.Counterparty.Name != "Omar K" {
			t.Fatalf("debt account = %+v", acc)
		}
		if !strings.Contains(body, acc.ID) {
			t.Errorf("option misses the account: %s", body)
		}
		// Asking again reuses it.
		do("POST", "/transactions/bought-by/person", url.Values{"name": {"omar k"}})
		accs, _ = repo.GetAccounts(ctx)
		n := 0
		for _, a := range accs {
			if a.Kind.Enum == models.AccKindDebt && a.Counterparty != nil && a.Counterparty.ID == acc.Counterparty.ID {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%d debt accounts for them", n)
		}
	})

	t.Run("the expense can't go below what its debts brought in", func(t *testing.T) {
		res, body := do("PUT", "/transactions/"+expense.ID, url.Values{
			"kind": {models.ExpenseTxKind.String()}, "account_id": {bank.ID}, "amount": {"200"},
			"scheduled_to": {today}, "description": {"Headphones"},
		})
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Can&#39;t be less than what the people who bought it paid") {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
	})

	t.Run("unlinking and linking an existing debt back", func(t *testing.T) {
		debtID := expense.BoughtBy[0].ID
		if res, body := do("DELETE", "/transactions/"+debtID+"/bought-for", nil); res.StatusCode != http.StatusOK {
			t.Fatalf("unlink status %d: %s", res.StatusCode, body)
		}
		if e := find("Headphones", models.ExpenseTxKind); len(e.BoughtBy) != 0 {
			t.Fatalf("still linked: %+v", e.BoughtBy)
		}

		res, body := do("GET", "/transactions/"+expense.ID+"/bought-by", nil)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, debtID) || !strings.Contains(body, "Likely") {
			t.Fatalf("picker status %d: %s", res.StatusCode, body)
		}
		if res, body := do("GET", "/transactions/"+expense.ID, nil); res.StatusCode != http.StatusOK || !strings.Contains(body, `id="tx-list"`) {
			t.Fatalf("cancel status %d: %s", res.StatusCode, body)
		}
		res, body = do("GET", "/transactions/"+debtID+"/bought-for", nil)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, expense.ID) {
			t.Fatalf("debt picker status %d: %s", res.StatusCode, body)
		}

		if res, body := do("PUT", "/transactions/"+debtID+"/bought-for", url.Values{"expense_id": {expense.ID}}); res.StatusCode != http.StatusOK {
			t.Fatalf("link status %d: %s", res.StatusCode, body)
		}
		if e := find("Headphones", models.ExpenseTxKind); len(e.BoughtBy) != 1 {
			t.Fatalf("not linked back: %+v", e.BoughtBy)
		}
	})

	t.Run("editing a linked debt keeps it linked", func(t *testing.T) {
		debtID := find("Headphones", models.ExpenseTxKind).BoughtBy[0].ID
		res, body := do("PUT", "/transactions/"+debtID, url.Values{
			"kind": {models.DebtTxKind.String()}, "account_id": {saraAcc.ID}, "to_account_id": {bank.ID},
			"amount": {"300"}, "to_amount": {"300"}, "scheduled_to": {today}, "description": {"Headphones gift"},
		})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		d, err := repo.GetTransaction(ctx, debtID)
		if err != nil {
			t.Fatal(err)
		}
		if d.BoughtForID != expense.ID || d.Description != "Headphones gift" {
			t.Errorf("debt = %q linked to %q", d.Description, d.BoughtForID)
		}
	})

	t.Run("debts can't be more than the expense", func(t *testing.T) {
		if err := repo.CreateTransaction(ctx, &models.Transaction{
			ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8D2", Kind: models.DebtTxKind, Account: *saraAcc, ToAccount: bank,
			Amount: 50, ToAmount: 50, Description: "More", ScheduledTo: time.Now(), FulfilledAt: time.Now(),
		}, nil, nil); err != nil {
			t.Fatal(err)
		}
		_, body := do("PUT", "/transactions/"+expense.ID+"/bought-by", url.Values{"debt_id": {"01K8KQZ4N1XQ5WJ5M3Q6G7Y8D2"}})
		if !strings.Contains(body, models.ErrBoughtForExceeds.Error()) {
			t.Errorf("answer = %s", body)
		}
	})

	t.Run("deleting the expense leaves the debt on its own", func(t *testing.T) {
		debtID := find("Headphones", models.ExpenseTxKind).BoughtBy[0].ID
		if err := repo.DeleteTransaction(ctx, expense); err != nil {
			t.Fatal(err)
		}
		d, err := repo.GetTransaction(ctx, debtID)
		if err != nil {
			t.Fatal(err)
		}
		if d.IsBoughtFor() {
			t.Errorf("debt still linked to %s", d.BoughtForID)
		}
		if s := balance(saraAcc.ID); s != -350 {
			t.Errorf("sara = %v, want -350", s)
		}
	})
}
