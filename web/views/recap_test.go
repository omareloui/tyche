package views

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
)

func TestMonthRecapRenders(t *testing.T) {
	egp := models.Currency{String: "EGP", Symbol: "E£"}
	p := models.MonthStart(1).For(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	cat := models.Category{ID: "food", Name: "Food", Color: "red"}
	rc := &models.MonthRecap{
		Period: p, Today: time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC), Currency: egp, PrevMonth: "2026-09", NextMonth: "2026-11",
		Flow: models.MonthFlow{In: 1000, Out: 600}, Prev: models.MonthFlow{In: 900, Out: 700}, Avg: models.MonthFlow{In: 950, Out: 500}, AvgMonths: 2,
		CategoryChanges: []models.RecapCategoryChange{{Category: &cat, Name: "Food", Color: "red", Amount: 300, Avg: 100}, {Name: "Uncategorized", Color: "gray", Amount: 0, Avg: 50}},
		NewMerchants:    &models.MerchantSpending{Period: p, Currency: egp, Items: []models.MerchantSpend{{Counterparty: models.Counterparty{ID: "x", Name: "Shop"}, Amount: 50}}},
		Largest:         []models.RecapTx{{Tx: models.Transaction{ID: "t", Description: "TV", Account: models.Account{Name: "Bank"}, Reimbursed: 5}, Amount: 400}},
		Budgets:         models.NewBudgetSummary(p, p.Last(), egp, []models.Category{{ID: "food", Name: "Food", BudgetAmount: 200, BudgetCurrency: &egp, Kinds: []models.TxKind{models.ExpenseTxKind}}}, nil, nil),
		Goals:           []models.RecapGoal{{Goal: models.Goal{ID: "g", Name: "Car", Currency: egp}, Deposited: 100}, {Goal: models.Goal{ID: "h", Name: "Phone"}, Achieved: true}},
		NetStart:        100, NetEnd: 500, NoSpendDays: 4, Busiest: models.DayFlow{Day: p.From, Out: 90},
	}
	var buf bytes.Buffer
	if err := MonthRecap(&jwt.AccessClaims{}, nil, rc).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"October", "Biggest category changes", "New merchants", "TV", "Car", "Phone", "/recap?month=2026-09"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %q", want)
		}
	}
	buf.Reset()
	if err := RecapBanner(p).Render(context.Background(), &buf); err != nil || !strings.Contains(buf.String(), "/recap?month=2026-10") {
		t.Errorf("banner: %v %s", err, buf.String())
	}
}
