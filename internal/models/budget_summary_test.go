package models

import (
	"slices"
	"testing"
)

func budgetSummaryFixture(today string, hidden []string) *BudgetSummary {
	egp := Currency{String: "EGP"}
	xyz := Currency{String: "XYZ"} // no rate
	cats := []Category{
		{ID: "food", Name: "Food", BudgetAmount: 1000, BudgetCurrency: &egp, Kinds: []TxKind{ExpenseTxKind}},
		{ID: "rent", Name: "Rent", BudgetAmount: 500, BudgetCurrency: &egp, Kinds: []TxKind{ExpenseTxKind}},
		{ID: "fun", Name: "Fun", Kinds: []TxKind{ExpenseTxKind}},
		{ID: "idle", Name: "Idle", Kinds: []TxKind{ExpenseTxKind}},
		{ID: "salary", Name: "Salary", Kinds: []TxKind{IncomeTxKind}},
	}
	return NewBudgetSummary(day("2026-10-01"), day(today), egp, cats, []DayCategoryTotal{
		{day("2026-10-02"), CategoryTotal{CategoryID: "food", Currency: egp, Amount: 300}},
		{day("2026-10-05"), CategoryTotal{CategoryID: "food", Currency: egp, Amount: 100}},
		{day("2026-10-03"), CategoryTotal{CategoryID: "rent", Currency: egp, Amount: 600}},
		{day("2026-10-04"), CategoryTotal{CategoryID: "fun", Currency: egp, Amount: 80}},
		{day("2026-10-04"), CategoryTotal{CategoryID: "", Currency: egp, Amount: 20}},
		{day("2026-10-04"), CategoryTotal{CategoryID: "salary", Currency: egp, Amount: 999}},
		{day("2026-10-06"), CategoryTotal{CategoryID: "fun", Currency: xyz, Amount: 5}},
	}, hidden)
}

func keys(items []BudgetSummaryItem) []string {
	var res []string
	for _, it := range items {
		res = append(res, it.Key())
	}
	return res
}

func TestNewBudgetSummary(t *testing.T) {
	bs := budgetSummaryFixture("2026-10-10", nil)
	// Budgeted most used first, then unbudgeted most spent; idle (no budget,
	// no spending) and income categories left out.
	if got, want := keys(bs.Items), []string{"rent", "food", "fun", UncategorizedKey}; !slices.Equal(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
	if !bs.Partial {
		t.Error("partial = false, want true")
	}
	if bs.Spent() != 1100 || bs.Budget() != 1500 || bs.BudgetedSpent() != 1000 {
		t.Errorf("spent = %v, budget = %v, budgeted spent = %v", bs.Spent(), bs.Budget(), bs.BudgetedSpent())
	}
	if bs.Over() != 1 || bs.DaysLeft() != 22 || !near(bs.MonthPct(), 10.0/31*100) {
		t.Errorf("over = %d, days left = %d, month pct = %v", bs.Over(), bs.DaysLeft(), bs.MonthPct())
	}
	// 1000 / 1500 used after 10 of 31 days.
	if !bs.Ahead() {
		t.Error("ahead = false, want true")
	}
	if safe, ok := bs.SafePerDay(); !ok || !near(safe, 600.0/22) {
		t.Errorf("safe per day = %v %v", safe, ok)
	}
	if got := keys(bs.Unbudgeted()); !slices.Equal(got, []string{"fun", UncategorizedKey}) || bs.UnbudgetedSpent() != 100 {
		t.Errorf("unbudgeted = %v, spent = %v", got, bs.UnbudgetedSpent())
	}
	cum := bs.Cumulative()
	if len(cum) != 10 || cum[0] != 0 || cum[1] != 300 || cum[2] != 900 || cum[4] != 1000 || cum[9] != 1000 {
		t.Errorf("cumulative = %v", cum)
	}
}

func TestBudgetSummaryHidden(t *testing.T) {
	bs := budgetSummaryFixture("2026-10-10", []string{"rent", UncategorizedKey})
	if got, want := keys(bs.Items), []string{"food", "fun", "rent", UncategorizedKey}; !slices.Equal(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
	if got := keys(bs.Hidden()); !slices.Equal(got, []string{"rent", UncategorizedKey}) {
		t.Errorf("hidden = %v", got)
	}
	if bs.Spent() != 480 || bs.Budget() != 1000 || bs.Over() != 0 {
		t.Errorf("spent = %v, budget = %v, over = %d", bs.Spent(), bs.Budget(), bs.Over())
	}
	// 40% used after 32% of the month.
	if !bs.Ahead() {
		t.Error("ahead = false, want true")
	}
	if cum := bs.Cumulative(); cum[2] != 300 || cum[9] != 400 {
		t.Errorf("cumulative = %v", cum)
	}
}

func TestBudgetSummaryPastMonth(t *testing.T) {
	bs := budgetSummaryFixture("2026-11-02", []string{"rent"})
	if bs.IsCurrent() || bs.MonthPct() != 100 || bs.Elapsed() != 31 || bs.DaysLeft() != 0 {
		t.Errorf("current = %v, month pct = %v, elapsed = %d, days left = %d", bs.IsCurrent(), bs.MonthPct(), bs.Elapsed(), bs.DaysLeft())
	}
	if bs.Ahead() {
		t.Error("ahead = true, want false in a past month under budget")
	}
	if _, ok := bs.SafePerDay(); ok {
		t.Error("safe per day ok in a past month")
	}
	if !near(bs.PaceAt(30), 1000) {
		t.Errorf("pace at last day = %v", bs.PaceAt(30))
	}
}
