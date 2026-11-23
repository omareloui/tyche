package models

import (
	"testing"
	"time"
)

func TestMonthDaysHeatmap(t *testing.T) {
	cairo, err := time.LoadLocation("Africa/Cairo")
	if err != nil {
		t.Fatal(err)
	}
	eet := time.FixedZone("", 2*3600)
	s := MonthStart(26)
	today := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	from, to := HeatmapPeriod(s, today, 3)
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	txs := []SpendingTime{
		// Payday: column 0 of November's month (Oct 26 – Nov 25).
		{At: time.Date(2026, 10, 26, 10, 0, 0, 0, eet), CategoryID: "food", Amount: 100},
		// Its last day.
		{At: time.Date(2026, 11, 25, 20, 0, 0, 0, eet), Amount: 50},
		// 23:30 UTC on Oct 27 is Oct 28 in Cairo.
		{At: time.Date(2026, 10, 27, 23, 30, 0, 0, time.UTC), Amount: 30},
		// Picked in a form: stays on its day.
		{At: day(10, 27), Amount: 20},
		// A refund nets the day without being counted.
		{At: time.Date(2026, 10, 26, 11, 0, 0, 0, eet), CategoryID: "food", Amount: -40},
		// Payday of September's month (Aug 26 – Sep 25).
		{At: time.Date(2026, 8, 26, 9, 0, 0, 0, eet), Amount: 70},
		// Out of range: dropped.
		{At: time.Date(2026, 8, 25, 9, 0, 0, 0, eet), Amount: 999},
	}
	hm := NewMonthDaysHeatmap(s, Currency{String: "EGP"}, HeatmapByAmount, 3, from, to, today, cairo, txs, false)

	if len(hm.Rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(hm.Rows))
	}
	if got := hm.Rows[0].Period.From; !got.Equal(day(8, 26)) {
		t.Errorf("first row from %v, want Aug 26", got)
	}
	nov := hm.Rows[2]
	if c := nov.Days[0]; c.Amount != 60 || c.Count != 1 || c.Refunded != 40 {
		t.Errorf("Oct 26 = %+v, want 60 net over 1 charge, 40 refunded", c.HeatSpend)
	}
	if c := nov.Days[len(nov.Days)-1]; c.Amount != 50 || !hm.Date(2, len(nov.Days)-1).Equal(day(11, 25)) {
		t.Errorf("last day = %+v on %v, want 50 on Nov 25", c.HeatSpend, hm.Date(2, len(nov.Days)-1))
	}
	if nov.Days[2].Amount != 30 || nov.Days[1].Amount != 20 {
		t.Errorf("Oct 27/28 = %v/%v, want 20/30 (UTC late night moved on)", nov.Days[1].Amount, nov.Days[2].Amount)
	}
	if hm.Total != 5 {
		t.Errorf("total = %d, want the 5 charges in range", hm.Total)
	}
	if c := hm.All[0]; c.Amount != 130 || c.Count != 2 {
		t.Errorf("All[0] = %+v, want both paydays (60 + 70)", c.HeatSpend)
	}
	// Sep 26 – Oct 25 has 30 days, the others 31.
	if got := len(hm.Rows[1].Days); got != 30 {
		t.Errorf("October's days = %d, want 30", got)
	}
	if hm.Columns() != 31 || hm.Has(1, 30) || !hm.Has(2, 30) {
		t.Errorf("columns = %d, has(Oct, 30) = %v", hm.Columns(), hm.Has(1, 30))
	}
	if hm.ColumnLabel(0) != 26 || hm.ColumnLabel(6) != 1 {
		t.Errorf("labels = %d, %d, want 26 and 1", hm.ColumnLabel(0), hm.ColumnLabel(6))
	}
	// Today is Nov 23: column 28 of November's month.
	if !hm.IsToday(2, 28) || hm.IsAhead(2, 28) || !hm.IsAhead(2, 29) {
		t.Error("today / ahead wrong")
	}
	if k, ok := hm.BusiestColumn(); !ok || k != 0 {
		t.Errorf("busiest column = %d", k)
	}
	if d, ok := hm.Peak(); !ok || !d.Equal(day(8, 26)) {
		t.Errorf("peak = %v, want Aug 26 (70)", d)
	}
	if top, _ := hm.TopCategories(nov.Days[0], 3); len(top) != 1 || top[0].Name != "Uncategorized" || top[0].Amount != 60 {
		t.Errorf("categories = %+v", top)
	}
}

func TestMonthDaysHeatmapShortMonth(t *testing.T) {
	// Starting on the 1st: February is 28 days, March 31.
	s := MonthStart(1)
	today := time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC)
	from, to := HeatmapPeriod(s, today, 2)
	hm := NewMonthDaysHeatmap(s, Currency{}, HeatmapByCount, 2, from, to, today, time.UTC, nil, false)
	if len(hm.Rows[0].Days) != 28 || hm.Columns() != 31 || hm.ColumnLabel(30) != 31 {
		t.Errorf("Feb %d days, %d columns, last label %d", len(hm.Rows[0].Days), hm.Columns(), hm.ColumnLabel(30))
	}
	if _, ok := hm.Peak(); ok {
		t.Error("peak without spending")
	}
}
