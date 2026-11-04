package router_test

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestAccountDetails(t *testing.T) {
	repo, ctx, _ := setupTenant(t)

	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	cash := newTestAccount(t, repo, ctx, "Wallet", "EGP", models.AccKindCash, 0)

	loc, err := repo.GetTimezone(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(loc)
	noon := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, loc)
	daysAgo := func(n int) time.Time { return noon.AddDate(0, 0, -n) }

	for _, tx := range []*models.Transaction{
		{Amount: 200, Kind: models.ExpenseTxKind, FulfilledAt: daysAgo(2)},
		{Amount: 100, Kind: models.TransferTxKind, ToAccount: cash, ToAmount: 100, FulfilledAt: daysAgo(1)},
		{Amount: 50, Kind: models.IncomeTxKind, FulfilledAt: daysAgo(0).Add(-time.Hour)},
		// scheduled: not in the balance yet
		{Amount: 30, Kind: models.ExpenseTxKind, ScheduledTo: noon.AddDate(0, 0, 3)},
	} {
		tx.ID = idgen.New()
		tx.Account = *bank
		if tx.ScheduledTo.IsZero() {
			tx.ScheduledTo = tx.FulfilledAt
		}
		if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
			t.Fatal(err)
		}
	}

	bank, err = repo.GetAccount(ctx, bank.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bank.Amount != 750 {
		t.Fatalf("bank balance = %v, want 750", bank.Amount)
	}

	h, err := repo.GetAccountHistory(ctx, bank)
	if err != nil {
		t.Fatal(err)
	}
	n := len(h.Balance)
	// End of day balances: 1000 before, 800 after the expense, 700 after
	// the transfer, 750 today.
	if got := h.Balance[n-4:]; got[0] != 1000 || got[1] != 800 || got[2] != 700 || got[3] != 750 {
		t.Errorf("last balances = %v, want 1000 800 700 750", got)
	}

	if cash, err = repo.GetAccount(ctx, cash.ID); err != nil {
		t.Fatal(err)
	}
	cashHist, err := repo.GetAccountHistory(ctx, cash)
	if err != nil {
		t.Fatal(err)
	}
	if got := cashHist.Balance[len(cashHist.Balance)-2:]; got[0] != 100 || got[1] != 100 {
		t.Errorf("cash balances = %v, want 100 100", got)
	}

	stats, err := repo.GetAccountTxStats(ctx, bank.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Count != 3 || stats.First.IsZero() || stats.Last.Before(stats.First) {
		t.Errorf("stats = %+v, want 3 txs", stats)
	}

	upcoming, err := repo.GetAccountUpcomingTransactions(ctx, bank.ID, noon.AddDate(0, 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(upcoming) != 1 || upcoming[0].Amount != 30 {
		t.Errorf("upcoming = %+v, want the 30 expense", upcoming)
	}

	// The cash account lists the transfer it got.
	recent, err := repo.GetAccountRecentTransactions(ctx, cash.ID, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].Kind != models.TransferTxKind {
		t.Errorf("cash recent = %+v, want the transfer", recent)
	}

	hd := handler.New(repo, valid.New(), sanitizer.New(), nil, nil)
	c, err := callHandler(ctx, hd.AccountDetails)(httptest.NewRecorder(), "GET", "/accounts/"+cash.ID+"/details", cash.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := c.Render(ctx, &b); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Wallet details", "from Bank", `data-url-param="acc"`} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("details miss %q", want)
		}
	}
}
