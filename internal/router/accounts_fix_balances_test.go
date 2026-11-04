package router_test

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestFixBalances(t *testing.T) {
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

	newBank := func(amount float64) *models.Account {
		t.Helper()
		a := &models.Account{ID: idgen.New(), Name: idgen.New(), Amount: amount, Currency: *egp, Kind: bankKind}
		if err := repo.CreateAccount(ctx, a); err != nil {
			t.Fatal(err)
		}
		return a
	}
	amountOf := func(id string) float64 {
		t.Helper()
		a, err := repo.GetAccount(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return a.Amount
	}
	adjustments := func(accID string) []models.Transaction {
		t.Helper()
		txs, err := repo.GetTransactions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var res []models.Transaction
		for _, tx := range txs {
			if tx.Kind == models.AdjustmentTxKind && tx.Account.ID == accID {
				res = append(res, tx)
			}
		}
		return res
	}

	t.Run("as transactions", func(t *testing.T) {
		up, down, same := newBank(100), newBank(100), newBank(100)
		changed, err := repo.FixBalances(ctx, []models.BalanceFix{
			{AccountID: up.ID, Amount: 150.25},
			{AccountID: down.ID, Amount: -20},
			{AccountID: same.ID, Amount: 100},
		}, true, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if changed != 2 {
			t.Errorf("changed = %d, want 2", changed)
		}
		for _, c := range []struct {
			acc        *models.Account
			want, diff float64
			txs        int
		}{{up, 150.25, 50.25, 1}, {down, -20, -120, 1}, {same, 100, 0, 0}} {
			if got := amountOf(c.acc.ID); got != c.want {
				t.Errorf("amount = %v, want %v", got, c.want)
			}
			txs := adjustments(c.acc.ID)
			if len(txs) != c.txs {
				t.Fatalf("adjustments = %d, want %d", len(txs), c.txs)
			}
			if c.txs == 1 && txs[0].Amount != c.diff {
				t.Errorf("adjustment = %v, want %v", txs[0].Amount, c.diff)
			}
		}

		// Deleting the adjustment undoes it.
		tx := adjustments(down.ID)[0]
		if err := repo.DeleteTransaction(ctx, &tx); err != nil {
			t.Fatal(err)
		}
		if got := amountOf(down.ID); got != 100 {
			t.Errorf("after delete: amount = %v, want 100", got)
		}
	})

	t.Run("as is", func(t *testing.T) {
		acc := newBank(100)
		changed, err := repo.FixBalances(ctx, []models.BalanceFix{{AccountID: acc.ID, Amount: 42.5}}, false, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if changed != 1 {
			t.Errorf("changed = %d, want 1", changed)
		}
		if got := amountOf(acc.ID); got != 42.5 {
			t.Errorf("amount = %v, want 42.5", got)
		}
		if txs := adjustments(acc.ID); len(txs) != 0 {
			t.Errorf("adjustments = %d, want 0", len(txs))
		}
	})

	t.Run("form", func(t *testing.T) {
		neg, big, blank := newBank(100), newBank(100), newBank(100)
		h := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)
		f := url.Values{
			"amount-" + neg.ID:   {"-1,250.50"},
			"amount-" + big.ID:   {"12,345.6"},
			"amount-" + blank.ID: {""},
			"mode":               {"set"},
		}
		r := httptest.NewRequest("POST", "/accounts/fix-balances", strings.NewReader(f.Encode())).WithContext(ctx)
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		c, err := h.FixBalances(httptest.NewRecorder(), r)
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		if err := c.Render(ctx, &b); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(b.String(), "Updated 2 accounts.") {
			t.Errorf("form doesn't say 2 accounts got updated")
		}
		for id, want := range map[string]float64{neg.ID: -1250.5, big.ID: 12345.6, blank.ID: 100} {
			if got := amountOf(id); got != want {
				t.Errorf("amount = %v, want %v", got, want)
			}
		}
		if txs := adjustments(neg.ID); len(txs) != 0 {
			t.Errorf("adjustments = %d, want 0 when set as is", len(txs))
		}
	})
}
