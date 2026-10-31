package router_test

import (
	"context"
	"io"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestCreateGamyaaSchedulesAndBalances(t *testing.T) {
	repo, ctx, _ := setupTenant(t)

	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	kinds, err := repo.GetAccountKinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var bankKind, gamyaaKind models.AccountKind
	for _, k := range kinds {
		switch k.Enum {
		case models.AccKindBankAcc:
			bankKind = k
		case models.AccKindGamyaa:
			gamyaaKind = k
		}
	}
	if gamyaaKind.ID == "" {
		t.Fatal("gam'yaa account kind isn't seeded")
	}

	bank := &models.Account{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZA", Name: "Bank", Amount: 10000, Currency: *egp, Kind: bankKind}
	if err := repo.CreateAccount(ctx, bank); err != nil {
		t.Fatal(err)
	}

	gam := &models.Account{
		ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZB", Name: "Office", Currency: *egp, Kind: gamyaaKind,
		Gamyaa: &models.Gamyaa{
			Installment: 1000, Members: 3, Shares: 0.5, Turns: []int{2},
			StartsOn: time.Date(2027, time.January, 10, 0, 0, 0, 0, time.UTC),
			PayFrom:  bank,
		},
	}
	if err := repo.CreateGamyaa(ctx, gam); err != nil {
		t.Fatal(err)
	}

	txs, err := repo.GetTransactions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) != 4 {
		t.Fatalf("got %d scheduled transactions, want 3 installments and a payout", len(txs))
	}

	got, err := repo.GetAccount(ctx, gam.ID)
	if err != nil {
		t.Fatal(err)
	}
	g := got.Gamyaa
	if g == nil {
		t.Fatal("gam'yaa not attached to its account")
	}
	if g.Members != 3 || g.Shares != 0.5 || len(g.Turns) != 1 || g.Turns[0] != 2 || g.PayFrom == nil || g.PayFrom.ID != bank.ID {
		t.Errorf("bad gam'yaa %+v", g)
	}
	if g.PaidCount != 0 || g.NextPaymentOn.Format(time.DateOnly) != "2027-01-10" || g.NextPayoutOn.Format(time.DateOnly) != "2027-02-10" {
		t.Errorf("bad progress %+v", g)
	}
	if g.RecurrentTxID == "" {
		t.Error("no recurrent tx")
	} else if rt, err := repo.GetRecurrentTx(ctx, g.RecurrentTxID); err != nil {
		t.Fatal(err)
	} else if len(rt.Upcoming) != 3 {
		t.Errorf("recurrent tx has %d upcoming, want 3", len(rt.Upcoming))
	}

	// Paying all and receiving the payout settles it.
	for _, tx := range txs {
		tx.FulfilledAt = tx.ScheduledTo
		if err := repo.FulfillTransaction(ctx, &tx); err != nil {
			t.Fatal(err)
		}
	}

	got, err = repo.GetAccount(ctx, gam.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Amount != 0 {
		t.Errorf("gam'yaa balance = %v, want 0 once done", got.Amount)
	}
	if !got.Gamyaa.IsDone() || got.Gamyaa.PaidCount != 3 || got.Gamyaa.PaidAmount != 1500 {
		t.Errorf("bad final progress %+v", got.Gamyaa)
	}
	b, err := repo.GetAccount(ctx, bank.ID)
	if err != nil {
		t.Fatal(err)
	}
	if b.Amount != 10000 {
		t.Errorf("bank balance = %v, want 10000 back", b.Amount)
	}
}

func TestCreateGamyaaFromForm(t *testing.T) {
	repo, ctx, _ := setupTenant(t)

	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	kinds, err := repo.GetAccountKinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var bankKind, gamyaaKind models.AccountKind
	for _, k := range kinds {
		switch k.Enum {
		case models.AccKindBankAcc:
			bankKind = k
		case models.AccKindGamyaa:
			gamyaaKind = k
		}
	}
	bank := &models.Account{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZA", Name: "Bank", Currency: *egp, Kind: bankKind}
	if err := repo.CreateAccount(ctx, bank); err != nil {
		t.Fatal(err)
	}

	h := handler.New(repo, valid.New(), sanitizer.New(), nil)
	post := func(path string, f url.Values) string {
		t.Helper()
		r := httptest.NewRequest("POST", path, strings.NewReader(f.Encode())).WithContext(ctx)
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		var c interface {
			Render(ctx context.Context, w io.Writer) error
		}
		var err error
		if path == "/accounts" {
			c, err = h.CreateAccount(w, r)
		} else {
			c, err = h.AccountInputs(w, r)
		}
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		if c != nil {
			if err := c.Render(ctx, &b); err != nil {
				t.Fatal(err)
			}
		}
		return w.Body.String() + b.String()
	}

	form := url.Values{
		"kind_id": {gamyaaKind.ID}, "name": {"Office"}, "currency_id": {egp.ID},
		"gamyaa_pay_from_id": {bank.ID}, "gamyaa_installment": {"E£ 1,000"},
		"gamyaa_members": {"3"}, "gamyaa_shares": {"1"}, "gamyaa_turns": {"2"},
		"gamyaa_starts_on": {"2027-05-10"},
	}

	// The turns' months follow the first payment's date.
	if out := post("/accounts-inputs/0", form); !strings.Contains(out, "Jun 27") || strings.Contains(out, "Nov 26") {
		t.Errorf("turns don't follow the start date")
	}

	post("/accounts", form)

	accs, err := repo.GetAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var gam *models.Account
	for i := range accs {
		if accs[i].Kind.Enum == models.AccKindGamyaa {
			gam = &accs[i]
		}
	}
	if gam == nil || gam.Gamyaa == nil {
		t.Fatal("gam'yaa not created")
	}
	if g := gam.Gamyaa; g.Installment != 1000 || g.Members != 3 || len(g.Turns) != 1 || g.NextPayoutOn.Format(time.DateOnly) != "2027-06-10" {
		t.Errorf("bad gam'yaa %+v", g)
	}
}
