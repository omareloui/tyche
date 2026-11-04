package models

import (
	"math"
	"testing"
	"time"
)

func day(s string) time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return d
}

func near(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestNewFlow(t *testing.T) {
	today := day("2026-10-15")
	f := NewFlow(1, today, []DayFlow{
		{Day: day("2026-04-30"), In: 999, Out: 999}, // before the window
		{Day: day("2026-05-02"), In: 100, Out: 50},
		{Day: day("2026-09-10"), In: 1000, Out: 200},
		{Day: day("2026-09-20"), Out: 300},
		{Day: day("2026-10-01"), In: 1000, Out: 100},
		{Day: day("2026-10-14"), Out: 150},
	}, false)

	if len(f.Months) != FlowMonths {
		t.Fatalf("months = %d, want %d", len(f.Months), FlowMonths)
	}
	if !f.Months[0].Month.Equal(day("2026-05-01")) || f.Months[0].In != 100 {
		t.Errorf("first month = %+v, want May with 100 in", f.Months[0])
	}
	if m := f.Months[2]; m.In != 0 || m.Out != 0 {
		t.Errorf("empty July = %+v, want zeros", m)
	}
	if cur := f.Current(); cur.In != 1000 || cur.Out != 250 {
		t.Errorf("current = %+v, want 1000 in, 250 out", cur)
	}
	if prev := f.Prev(); prev.Out != 500 {
		t.Errorf("prev out = %v, want 500", prev.Out)
	}
	// By the 15th of September only the 10th counts.
	if p := f.PrevToDate(); p.In != 1000 || p.Out != 200 {
		t.Errorf("prev to date = %+v, want 1000 in, 200 out", p)
	}
	if r, ok := f.Current().SavingsRate(); !ok || !near(r, 75) {
		t.Errorf("savings rate = %v %v, want 75", r, ok)
	}

	cur, prev := f.Pace()
	if len(cur) != 15 || len(prev) != 30 {
		t.Fatalf("pace lens = %d, %d, want 15, 30", len(cur), len(prev))
	}
	if cur[0] != 100 || cur[12] != 100 || cur[13] != 250 || cur[14] != 250 {
		t.Errorf("cur pace = %v", cur)
	}
	if prev[8] != 0 || prev[9] != 200 || prev[19] != 500 || prev[29] != 500 {
		t.Errorf("prev pace = %v", prev)
	}
}

func TestPrevToDateShorterMonth(t *testing.T) {
	// On March 31st, all of February counts.
	f := NewFlow(1, day("2026-03-31"), []DayFlow{{Day: day("2026-02-28"), Out: 10}}, false)
	if p := f.PrevToDate(); p.Out != 10 {
		t.Errorf("prev to date out = %v, want 10", p.Out)
	}
}

func TestNewFlowMonthStart(t *testing.T) {
	// Months start on the 25th: today (Nov 2) is in November (Oct 25 – Nov 24).
	f := NewFlow(25, day("2026-11-02"), []DayFlow{
		{Day: day("2026-09-24"), Out: 1},  // September
		{Day: day("2026-09-25"), Out: 10}, // October
		{Day: day("2026-09-27"), Out: 20},
		{Day: day("2026-10-24"), Out: 40},
		{Day: day("2026-10-25"), In: 5000}, // November
		{Day: day("2026-10-26"), Out: 100},
		{Day: day("2026-11-02"), Out: 50},
	}, false)
	cur, prev := f.Current(), f.Prev()
	if !cur.Month.Equal(day("2026-11-01")) || cur.In != 5000 || cur.Out != 150 {
		t.Errorf("current = %+v", cur)
	}
	if !prev.Month.Equal(day("2026-10-01")) || prev.Out != 70 {
		t.Errorf("prev = %+v", prev)
	}
	if f.Months[3].Out != 1 {
		t.Errorf("september = %+v", f.Months[3])
	}
	if got := FlowFrom(25, day("2026-11-02")); !got.Equal(day("2026-05-25")) {
		t.Errorf("FlowFrom = %v", got)
	}
	// 9 days passed (Oct 25 – Nov 2): Sep 25 – Oct 3 of last month.
	if p := f.PrevToDate(); p.Out != 30 {
		t.Errorf("prev to date = %+v, want 30 out", p)
	}
	curPace, prevPace := f.Pace()
	if len(curPace) != 9 || len(prevPace) != 30 {
		t.Fatalf("pace lens = %d, %d, want 9, 30", len(curPace), len(prevPace))
	}
	if curPace[1] != 100 || curPace[8] != 150 || prevPace[29] != 70 {
		t.Errorf("pace = %v, %v", curPace, prevPace)
	}
	b := NewBudgets(25, day("2026-11-02"), Currency{String: "EGP"}, nil, nil)
	if b.DaysLeft() != 23 || !near(b.MonthPct(), 9.0/31*100) {
		t.Errorf("days left = %d, month pct = %v", b.DaysLeft(), b.MonthPct())
	}
}

func TestPctChange(t *testing.T) {
	if _, ok := PctChange(10, 0); ok {
		t.Error("PctChange from 0 should not be ok")
	}
	if p, _ := PctChange(150, 100); !near(p, 50) {
		t.Errorf("PctChange(150, 100) = %v, want 50", p)
	}
}

func TestNewCategorySpending(t *testing.T) {
	egp := Currency{String: "EGP"}
	cats := []Category{{ID: "food", Name: "Food", Color: "red"}, {ID: "fun", Name: "Fun", Color: "blue"}}
	cs := NewCategorySpending(MonthStart(1).For(day("2026-10-01")), egp, cats, []CategoryTotal{
		{CategoryID: "food", Currency: egp, Amount: 300},
		{CategoryID: "fun", Currency: egp, Amount: -20}, // refunds cover it
		{CategoryID: "", Currency: egp, Amount: 100},
	})
	if len(cs.Items) != 2 {
		t.Fatalf("items = %+v, want food and uncategorized", cs.Items)
	}
	if cs.Items[0].Name != "Food" || cs.Items[1].Name != "Uncategorized" || cs.Items[1].Category != nil {
		t.Errorf("items = %+v", cs.Items)
	}
	if cs.Total != 400 || !near(cs.Pct(300), 75) || cs.Max() != 300 {
		t.Errorf("total = %v, pct = %v, max = %v", cs.Total, cs.Pct(300), cs.Max())
	}
}

func TestNewCategorySpendingFoldsOther(t *testing.T) {
	egp := Currency{String: "EGP"}
	var totals []CategoryTotal
	for i := range MaxCategorySpends + 2 {
		totals = append(totals, CategoryTotal{CategoryID: string(rune('a' + i)), Currency: egp, Amount: float64(100 - i)})
	}
	cs := NewCategorySpending(MonthStart(1).For(day("2026-10-01")), egp, nil, totals)
	if len(cs.Items) != MaxCategorySpends {
		t.Fatalf("items = %d, want %d", len(cs.Items), MaxCategorySpends)
	}
	other := cs.Items[len(cs.Items)-1]
	// The last 3 (95, 94, 93) fold into Other.
	if other.Name != "Other" || other.Amount != 95+94+93 {
		t.Errorf("other = %+v", other)
	}
}

func TestNewBudgets(t *testing.T) {
	egp := Currency{String: "EGP"}
	cats := []Category{
		{ID: "food", Name: "Food", BudgetAmount: 1000, BudgetCurrency: &egp, Kinds: []TxKind{ExpenseTxKind}},
		{ID: "rent", Name: "Rent", BudgetAmount: 500, BudgetCurrency: &egp, Kinds: []TxKind{ExpenseTxKind}},
		{ID: "salary", Name: "Salary", BudgetAmount: 500, BudgetCurrency: &egp, Kinds: []TxKind{IncomeTxKind}},
		{ID: "misc", Name: "Misc", Kinds: []TxKind{ExpenseTxKind}},
	}
	b := NewBudgets(1, day("2026-10-21"), egp, cats, []CategoryTotal{
		{CategoryID: "food", Currency: egp, Amount: 400},
		{CategoryID: "rent", Currency: egp, Amount: 600},
		{CategoryID: "misc", Currency: egp, Amount: 50},
	})
	if len(b.Items) != 2 || b.Items[0].Category.ID != "rent" {
		t.Fatalf("items = %+v, want rent then food", b.Items)
	}
	if b.Over() != 1 || b.DaysLeft() != 11 {
		t.Errorf("over = %d, days left = %d", b.Over(), b.DaysLeft())
	}
	// Only food has some left: 600 over 11 days.
	if safe, ok := b.SafePerDay(); !ok || !near(safe, 600.0/11) {
		t.Errorf("safe per day = %v %v", safe, ok)
	}
}
