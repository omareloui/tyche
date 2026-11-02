package router_test

import (
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
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
