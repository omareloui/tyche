package models

import (
	"slices"
	"testing"
)

func TestRecapPeriod(t *testing.T) {
	s := MonthStart(25)
	today := day("2026-11-03") // in November (Oct 25 – Nov 24)
	for _, tt := range []struct {
		month, want string
		ok          bool
	}{
		{"", "2026-10", true},        // the last closed one
		{"2026-08", "2026-08", true}, // any past one
		{"2026-11", "2026-11", true}, // the open one
		{"2027-02", "2026-11", true}, // never ahead
		{"nope", "", false},
	} {
		p, ok := RecapPeriod(s, tt.month, today)
		if ok != tt.ok || (ok && p.Key() != tt.want) {
			t.Errorf("RecapPeriod(%q) = %s, %v; want %s, %v", tt.month, p.Key(), ok, tt.want, tt.ok)
		}
	}
}

func TestRecapAvgPeriods(t *testing.T) {
	p := MonthStart(1).For(day("2026-10-01"))
	keysOf := func(ps []Period) []string {
		var res []string
		for _, q := range ps {
			res = append(res, q.Key())
		}
		return res
	}
	if got := keysOf(RecapAvgPeriods(p, day("2025-01-10"))); !slices.Equal(got, []string{"2026-09", "2026-08", "2026-07"}) {
		t.Errorf("long history: %v", got)
	}
	// Data since August 20: August counts, July doesn't.
	if got := keysOf(RecapAvgPeriods(p, day("2026-08-20"))); !slices.Equal(got, []string{"2026-09", "2026-08"}) {
		t.Errorf("short history: %v", got)
	}
	if got := RecapAvgPeriods(p, day("2026-10-02")); len(got) != 0 {
		t.Errorf("first month: %v", keysOf(got))
	}
	if got := RecapAvgPeriods(p, Period{}.From); len(got) != 0 {
		t.Errorf("no data: %v", keysOf(got))
	}
}

func TestNewRecapFlow(t *testing.T) {
	p := MonthStart(1).For(day("2026-10-01"))
	avg := RecapAvgPeriods(p, day("2026-08-01")) // September, August
	cur, prev, mean := NewRecapFlow(p, avg, []DayFlow{
		{Day: day("2026-07-30"), In: 999, Out: 999}, // before the average
		{Day: day("2026-08-05"), In: 1000, Out: 400},
		{Day: day("2026-09-05"), In: 1000, Out: 800},
		{Day: day("2026-10-01"), In: 1200, Out: 300},
		{Day: day("2026-10-31"), Out: 200},
		{Day: day("2026-11-01"), Out: 999}, // after
	})
	if cur.In != 1200 || cur.Out != 500 {
		t.Errorf("cur = %+v", cur)
	}
	if prev.In != 1000 || prev.Out != 800 || prev.Period.Key() != "2026-09" {
		t.Errorf("prev = %+v", prev)
	}
	if mean.In != 1000 || mean.Out != 600 {
		t.Errorf("avg = %+v, want 1000 in / 600 out over 2 months", mean)
	}
	if rate, ok := cur.SavingsRate(); !ok || rate < 58.3 || rate > 58.4 {
		t.Errorf("savings rate = %v, %v", rate, ok)
	}
}

func TestRecapDays(t *testing.T) {
	p := MonthStart(1).For(day("2026-11-01"))
	days := []DayFlow{
		{Day: day("2026-11-01"), Out: 50},
		{Day: day("2026-11-02"), In: 1000}, // income only: no spending
		{Day: day("2026-11-03"), Out: 120},
		{Day: day("2026-11-09"), Out: 500}, // after today
	}
	noSpend, busiest := RecapDays(p, day("2026-11-05"), days)
	if noSpend != 3 {
		t.Errorf("no-spend days = %d, want 3 of 5", noSpend)
	}
	if !busiest.Day.Equal(day("2026-11-03")) {
		t.Errorf("busiest = %v", busiest.Day)
	}
}

func TestNewRecapCategoryChanges(t *testing.T) {
	egp := Currency{String: "EGP"}
	p := MonthStart(1).For(day("2026-10-01"))
	avg := RecapAvgPeriods(p, day("2026-01-01"))
	cats := []Category{
		{ID: "food", Name: "Food", Color: "red"},
		{ID: "rent", Name: "Rent", Color: "blue"},
	}
	tot := func(d, cat string, amount float64) DayCategoryTotal {
		return DayCategoryTotal{day(d), CategoryTotal{CategoryID: cat, Currency: egp, Amount: amount}}
	}
	got, partial := NewRecapCategoryChanges(p, avg, egp, cats, []DayCategoryTotal{
		// Food: 300 a month before, 900 now (+600).
		tot("2026-07-05", "food", 300), tot("2026-08-05", "food", 300), tot("2026-09-05", "food", 300),
		tot("2026-10-05", "food", 900),
		// Rent: the same every month, no change.
		tot("2026-07-01", "rent", 500), tot("2026-08-01", "rent", 500), tot("2026-09-01", "rent", 500),
		tot("2026-10-01", "rent", 500),
		// Uncategorized: 150 in August only (avg 50), none now (−50).
		tot("2026-08-10", "", 150),
		// Ignored: before the average.
		tot("2026-06-10", "food", 10000),
		{day("2026-10-06"), CategoryTotal{CategoryID: "food", Currency: Currency{String: "XYZ"}, Amount: 1}},
	})
	if !partial {
		t.Error("a currency without a rate isn't partial")
	}
	if len(got) != 2 {
		t.Fatalf("changes = %+v, want food and uncategorized", got)
	}
	if got[0].Name != "Food" || got[0].Change() != 600 || got[0].Avg != 300 {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].Category != nil || got[1].Name != "Uncategorized" || got[1].Change() != -50 {
		t.Errorf("second = %+v", got[1])
	}
}

func TestLargestExpenses(t *testing.T) {
	egp := Currency{String: "EGP"}
	acc := Account{Currency: egp}
	txs := []Transaction{
		{ID: "dinner", Kind: ExpenseTxKind, Account: acc, Amount: 1000, Reimbursed: 700}, // net 300
		{ID: "tv", Kind: ExpenseTxKind, Account: acc, Amount: 800},
		{ID: "leg", Kind: ExpenseTxKind, Account: acc, Amount: 5000, TransferPairID: "in"},
		{ID: "salary", Kind: IncomeTxKind, Account: acc, Amount: 9000},
		{ID: "gift", Kind: ExpenseTxKind, Account: acc, Amount: 500},
		{ID: "paid back", Kind: ExpenseTxKind, Account: acc, Amount: 200, Reimbursed: 200},
		{ID: "coffee", Kind: ExpenseTxKind, Account: acc, Amount: 50},
	}
	got, _ := LargestExpenses(txs, egp, 3)
	var ids []string
	for _, it := range got {
		ids = append(ids, it.Tx.ID)
	}
	if !slices.Equal(ids, []string{"tv", "gift", "dinner"}) {
		t.Errorf("largest = %v, want tv, gift, dinner (net 300)", ids)
	}
	if got[2].Amount != 300 {
		t.Errorf("dinner = %v, want its net 300", got[2].Amount)
	}
}

func TestRecapNetWorth(t *testing.T) {
	egp := Currency{String: "EGP"}
	p := MonthStart(1).For(day("2026-10-01"))
	accs := []Account{
		{ID: "bank", Currency: egp, Amount: 1000},
		{ID: "excluded", Currency: egp, Amount: 5000, ExcludeFromBalance: true},
		{ID: "goal", Currency: egp, Amount: 700, Kind: AccountKind{Enum: AccKindGoal}},
	}
	daily := map[string][]DayFlow{
		"bank": {
			{Day: day("2026-10-02"), In: 500},
			{Day: day("2026-10-20"), Out: 200},
			{Day: day("2026-11-03"), Out: 100}, // after the month
		},
		"excluded": {{Day: day("2026-10-02"), In: 5000}},
	}
	start, end, partial := RecapNetWorth(accs, egp, daily, p)
	if partial || end != 1100 || start != 800 {
		t.Errorf("net worth %v → %v (partial %v), want 800 → 1100", start, end, partial)
	}
}

func TestNewRecapGoals(t *testing.T) {
	p := MonthStart(1).For(day("2026-10-01"))
	goals := []Goal{
		{ID: "car", Name: "Car"},
		{ID: "phone", Name: "Phone", FulfilledAt: day("2026-10-15")},
		{ID: "trip", Name: "Trip"},
		{ID: "old", Name: "Old", FulfilledAt: day("2026-05-01")},
	}
	got := NewRecapGoals(goals, map[string]float64{"car": 500, "trip": 900}, p)
	var ids []string
	for _, g := range got {
		ids = append(ids, g.Goal.ID)
	}
	if !slices.Equal(ids, []string{"phone", "trip", "car"}) {
		t.Errorf("goals = %v, want the bought one first then the most put in", ids)
	}
}
