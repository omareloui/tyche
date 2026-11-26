package models

import (
	"testing"
	"time"
)

func tagSummaryForTest(t *testing.T) *TagSummary {
	t.Helper()
	egp := Currency{String: "EGP"}
	cats := map[string]Category{
		"food": {ID: "food", Name: "Food"},
		"taxi": {ID: "taxi", Name: "Taxi"},
	}
	tags := []Tag{{ID: "work", Name: "Work"}}
	spends := []TagSpend{
		{TagID: "work", Day: "2026-09-28", CategoryID: "food", Currency: egp, Amount: 40},
		{TagID: "work", Day: "2026-10-26", CategoryID: "food", Currency: egp, Amount: 100},
		{TagID: "work", Day: "2026-10-26", CategoryID: "taxi", Currency: egp, Amount: 50},
		{TagID: "work", Day: "2026-10-27", CategoryID: "", Currency: egp, Amount: 10},
		{TagID: "work", Day: "2026-11-03", CategoryID: "taxi", Currency: egp, Amount: 30},
	}
	return NewTagSummaries(tags, spends, cats, egp)["work"]
}

func TestTagSummaryDayCategories(t *testing.T) {
	s := tagSummaryForTest(t)
	for _, d := range s.ByDay {
		var sum float64
		for _, c := range d.ByCategory {
			sum += c.Amount
		}
		if sum != d.Amount {
			t.Errorf("%s: categories sum %v, day %v", d.Day.Format(time.DateOnly), sum, d.Amount)
		}
	}
	// Days keep the summary's category order: food (140) before taxi (80).
	d := s.ByDay[1]
	if len(d.ByCategory) != 2 || d.ByCategory[0].Category.ID != "food" || d.ByCategory[1].Category.ID != "taxi" {
		t.Errorf("day categories = %+v", d.ByCategory)
	}
}

func TestTagSummaryMonth(t *testing.T) {
	s := tagSummaryForTest(t)
	start := MonthStart(25)
	p := start.For(day("2026-11-01")) // Oct 25 – Nov 24
	m := s.Month(p, time.Time{}, time.Time{})
	if len(m.Days) != p.Days() || m.Days[0].Day != p.From {
		t.Fatalf("days = %d from %v, want %d from %v", len(m.Days), m.Days[0].Day, p.Days(), p.From)
	}
	if m.Month.Amount != 190 || m.Max != 150 || m.Selection != nil {
		t.Errorf("month = %v, max %v, sel %v", m.Month.Amount, m.Max, m.Selection)
	}
	if len(m.Month.ByCategory) != 3 || m.Month.ByCategory[0].Category.ID != "food" || m.Month.ByCategory[1].Category.ID != "taxi" || m.Month.ByCategory[2].Category != nil {
		t.Errorf("month categories = %+v", m.Month.ByCategory)
	}
	if m.Prev != "2026-10" || m.Next != "" {
		t.Errorf("prev/next = %q/%q", m.Prev, m.Next)
	}
	if len(m.Months) != 2 || m.Months[0].Period.Key() != "2026-11" || m.Months[1].Amount != 40 {
		t.Errorf("months = %+v", m.Months)
	}

	// Reversed and past the month's end: clamped and swapped.
	m = s.Month(p, day("2026-12-30"), day("2026-10-27"))
	if m.Selection == nil || m.Selection.From != day("2026-10-27") || m.Selection.To != p.Last() {
		t.Fatalf("selection = %+v", m.Selection)
	}
	if m.Selection.Amount != 40 || m.Selection.Days() != p.Days()-2 || len(m.Selection.ByCategory) != 2 || m.Selection.ByCategory[0].Category.ID != "taxi" {
		t.Errorf("selection = %+v", m.Selection)
	}
	if m.Shown() != m.Selection || !m.Selected(day("2026-11-03")) || m.Selected(day("2026-10-26")) {
		t.Error("selected days wrong")
	}

	// Outside the months it spent in.
	m = s.Month(start.For(day("2027-01-01")), time.Time{}, time.Time{})
	if m.Month.Amount != 0 || m.Prev != "2026-12" || m.Next != "" || m.Months[0].Period.Key() != "2027-01" {
		t.Errorf("empty month = %v prev %q next %q", m.Month.Amount, m.Prev, m.Next)
	}
}

func TestTagSummaryDefaultMonth(t *testing.T) {
	s := tagSummaryForTest(t)
	start := MonthStart(25)
	if got := s.DefaultMonth(start, day("2026-11-10")).Key(); got != "2026-11" {
		t.Errorf("spent this month: %s", got)
	}
	if got := s.DefaultMonth(start, day("2027-03-01")).Key(); got != "2026-11" {
		t.Errorf("last spending's month: %s", got)
	}
	empty := &TagSummary{}
	if got := empty.DefaultMonth(start, day("2027-03-01")).Key(); got != "2027-03" {
		t.Errorf("none: %s", got)
	}
}
