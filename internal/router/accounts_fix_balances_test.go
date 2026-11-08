package router_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
	"github.com/omareloui/tyche/web/views"
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
	var bankKind, cardKind models.AccountKind
	for _, k := range kinds {
		switch k.Enum {
		case models.AccKindBankAcc:
			bankKind = k
		case models.AccKindCreditCard:
			cardKind = k
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
		}, time.Now())
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

	t.Run("set as is", func(t *testing.T) {
		acc := newBank(100)
		changed, err := repo.SetBalances(ctx, []models.BalanceFix{{AccountID: acc.ID, Amount: 42.5}})
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

	h := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)
	post := func(path string, f url.Values) *http.Request {
		r := httptest.NewRequest("POST", path, strings.NewReader(f.Encode())).WithContext(ctx)
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return r
	}
	render := func(c templ.Component) string {
		t.Helper()
		if c == nil {
			return ""
		}
		var b strings.Builder
		if err := c.Render(ctx, &b); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}

	t.Run("form", func(t *testing.T) {
		neg, big, blank := newBank(100), newBank(100), newBank(100)
		w := httptest.NewRecorder()
		c, err := h.FixBalances(w, post("/accounts/fix-balances", url.Values{
			"amount-" + neg.ID:   {egp.Symbol + " -1,250.50"},
			"amount-" + big.ID:   {"12,345.6"},
			"amount-" + blank.ID: {egp.Symbol + " "},
		}))
		if err != nil {
			t.Fatal(err)
		}
		render(c)
		if !strings.Contains(w.Header().Get("HX-Trigger"), views.AccountsChangedEvent) {
			t.Errorf("HX-Trigger = %q, want %s", w.Header().Get("HX-Trigger"), views.AccountsChangedEvent)
		}
		for _, id := range []string{neg.ID, big.ID} {
			if !strings.Contains(w.Body.String(), views.AccountWrapperID(id)) {
				t.Errorf("changed card %s not answered out of band", id)
			}
		}
		if strings.Contains(w.Body.String(), views.AccountWrapperID(blank.ID)) {
			t.Errorf("unchanged card answered")
		}
		for id, want := range map[string]float64{neg.ID: -1250.5, big.ID: 12345.6, blank.ID: 100} {
			if got := amountOf(id); got != want {
				t.Errorf("amount = %v, want %v", got, want)
			}
		}
		if txs := adjustments(neg.ID); len(txs) != 1 || txs[0].Amount != -1350.5 {
			t.Errorf("adjustments = %+v, want one of -1350.5", txs)
		}

		// Nothing changed: the form again, saying so.
		c, err = h.FixBalances(httptest.NewRecorder(), post("/accounts/fix-balances", url.Values{"amount-" + neg.ID: {"-1,250.50"}}))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(render(c), "Nothing changed.") {
			t.Errorf("form doesn't say nothing changed")
		}
	})

	t.Run("adjust one", func(t *testing.T) {
		acc := newBank(100)
		adjust := func(amount string) (*httptest.ResponseRecorder, string) {
			t.Helper()
			r := post("/accounts/"+acc.ID+"/adjust", url.Values{"amount": {amount}})
			r.SetPathValue("id", acc.ID)
			w := httptest.NewRecorder()
			c, err := h.AdjustBalance(w, r)
			if err != nil {
				t.Fatal(err)
			}
			return w, render(c)
		}

		if _, body := adjust(egp.Symbol + " "); !strings.Contains(body, "Invalid balance") {
			t.Errorf("invalid amount not refused")
		}
		if w, _ := adjust("100.00"); !strings.Contains(w.Header().Get("HX-Trigger"), "drawer-saved") {
			t.Errorf("same balance doesn't close the drawer")
		}
		if txs := adjustments(acc.ID); len(txs) != 0 {
			t.Errorf("adjustments = %d, want 0 for the same balance", len(txs))
		}

		w, _ := adjust(egp.Symbol + " 1,075.5")
		if got := w.Header().Get("HX-Retarget"); got != "#"+views.AccountWrapperID(acc.ID) {
			t.Errorf("HX-Retarget = %q, want the card", got)
		}
		if got := amountOf(acc.ID); got != 1075.5 {
			t.Errorf("amount = %v, want 1075.5", got)
		}
		if txs := adjustments(acc.ID); len(txs) != 1 || txs[0].Amount != 975.5 {
			t.Errorf("adjustments = %+v, want one of 975.5", txs)
		}
	})

	t.Run("adjust a card's debt", func(t *testing.T) {
		card := &models.Account{ID: idgen.New(), Name: idgen.New(), Amount: -100, Currency: *egp, Kind: cardKind}
		if err := repo.CreateAccount(ctx, card); err != nil {
			t.Fatal(err)
		}
		r := post("/accounts/"+card.ID+"/adjust", url.Values{"amount": {egp.Symbol + " -250"}})
		r.SetPathValue("id", card.ID)
		if _, err := h.AdjustBalance(httptest.NewRecorder(), r); err != nil {
			t.Fatal(err)
		}
		if got := amountOf(card.ID); got != -250 {
			t.Errorf("amount = %v, want -250 (250 owed)", got)
		}
	})
}
