package models

import (
	"testing"
	"time"
)

func TestSpendingHeatmapBuckets(t *testing.T) {
	cairo, err := time.LoadLocation("Africa/Cairo")
	if err != nil {
		t.Fatal(err)
	}
	eet := time.FixedZone("", 3*3600)
	txs := []SpendingTime{
		// SMS at 21:30 in the user's zone (Thursday).
		{At: time.Date(2026, 10, 22, 21, 30, 0, 0, eet), Amount: 100},
		// Stored in UTC (Thursday 23:10) is Friday 02:10 in Cairo (+03:00).
		{At: time.Date(2026, 10, 22, 23, 10, 0, 0, time.UTC), Amount: 40},
		// A day picked in a form: midnight UTC, counted on its day only.
		{At: time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC), Amount: 500},
		// A refund nets the hour.
		{At: time.Date(2026, 10, 22, 21, 5, 0, 0, eet), Amount: -30},
	}
	hm := NewSpendingHeatmap(Currency{String: "EGP"}, HeatmapByAmount, 1, time.Time{}, time.Time{}, time.Saturday, cairo, txs, false)

	if hm.Total != 3 || hm.Timed != 2 {
		t.Errorf("total/timed = %d/%d, want 3/2 (the refund not counted)", hm.Total, hm.Timed)
	}
	if c := hm.Cells[time.Thursday][21]; c.Amount != 70 || c.Count != 1 || c.Refunded != 30 {
		t.Errorf("Thu 21 = %+v, want 70 net over 1 charge, 30 refunded", c.HeatSpend)
	}
	if c := hm.Cells[time.Friday][2]; c.Amount != 40 || c.Count != 1 {
		t.Errorf("Fri 02 = %+v, want the UTC time moved to Cairo", c)
	}
	if c := hm.Cells[time.Thursday][0]; c.Count != 0 {
		t.Errorf("Thu 00 = %+v, want the midnight tx out of the hours", c)
	}
	if c := hm.Weekdays[time.Thursday]; c.Amount != 570 || c.Count != 2 {
		t.Errorf("Thu = %+v, want 570 over 2 charges (the midnight one included)", c.HeatSpend)
	}
	if hm.Hours[21].Count != 1 || hm.Hours[2].Count != 1 {
		t.Errorf("hours = %+v", hm.Hours)
	}
	if got := hm.Days(); got[0] != time.Saturday || got[6] != time.Friday {
		t.Errorf("days = %v, want Saturday first", got)
	}
	if hm.FirstHour() != 2 {
		t.Errorf("first hour = %d, want 2", hm.FirstHour())
	}
	if wd, col, ok := hm.Busiest(1); !ok || wd != time.Thursday || col.From != 21 {
		t.Errorf("busiest = %v %v %v, want Thu 21", wd, col, ok)
	}
}

func TestSpendingLocalTimeKeepsStoredZone(t *testing.T) {
	cairo, _ := time.LoadLocation("Africa/Cairo")
	// Written in summer time (+03:00), read in winter: stays 10:00.
	at := time.Date(2026, 7, 1, 10, 0, 0, 0, time.FixedZone("", 3*3600))
	if got, ok := SpendingLocalTime(at, cairo); !ok || got.Hour() != 10 {
		t.Errorf("got %v %v, want 10:00", got, ok)
	}
}

func TestSpendingHeatmapLevelsAndColumns(t *testing.T) {
	hm := &SpendingHeatmap{HeatScale: HeatScale{Metric: HeatmapByCount}}
	hm.Cells[time.Monday][9] = HeatCell{HeatSpend: HeatSpend{Amount: 10, Count: 4}}
	hm.Cells[time.Monday][10] = HeatCell{HeatSpend: HeatSpend{Amount: 10, Count: 1}}
	hm.Hours[9] = HeatCell{HeatSpend: HeatSpend{Count: 4}}
	hm.Hours[10] = HeatCell{HeatSpend: HeatSpend{Count: 1}}

	m := hm.Max(1)
	if m != 4 {
		t.Fatalf("max = %v, want 4", m)
	}
	if l := hm.Level(hm.Cells[time.Monday][9], m); l != HeatmapLevels-1 {
		t.Errorf("max level = %d", l)
	}
	if l := hm.Level(HeatCell{}, m); l != 0 {
		t.Errorf("empty level = %d", l)
	}
	if l := hm.Level(hm.Cells[time.Monday][10], m); l < 1 || l >= HeatmapLevels-1 {
		t.Errorf("small level = %d", l)
	}
	if cols := hm.Columns(1); cols[0].From != 7 || len(cols) != 17 {
		t.Errorf("cols = %v, want the empty night hours left out", cols)
	}
	cols := hm.Columns(3)
	if len(cols) != 8 || hm.Cell(time.Monday, cols[3]).Count != 5 {
		t.Errorf("3-hour cols = %v, Mon 09–12 = %+v", cols, hm.Cell(time.Monday, cols[3]))
	}
	if cols[7].RangeLabel() != "21–24" {
		t.Errorf("label = %q", cols[7].RangeLabel())
	}
}

func TestHeatmapPeriod(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	from, to := HeatmapPeriod(25, now, 3)
	if want := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC); !from.Equal(want) {
		t.Errorf("from = %v, want %v", from, want)
	}
	if want := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC); !to.Equal(want) {
		t.Errorf("to = %v, want %v", to, want)
	}
}

func TestSpendingHeatmapTopCategories(t *testing.T) {
	eet := time.FixedZone("", 3*3600)
	at := func(h, m int) time.Time { return time.Date(2026, 10, 22, h, m, 0, 0, eet) }
	txs := []SpendingTime{
		{At: at(21, 0), CategoryID: "food", Amount: 300},
		{At: at(21, 10), CategoryID: "food", Amount: 100},
		{At: at(21, 20), CategoryID: "fun", Amount: 250},
		{At: at(21, 30), CategoryID: "", Amount: 50},
		{At: at(21, 40), CategoryID: "cab", Amount: 30},
		{At: at(21, 50), CategoryID: "cab", Amount: 20},
		{At: at(21, 55), CategoryID: "cab", Amount: 10},
		// Refunded more than spent: left out.
		{At: at(21, 59), CategoryID: "gone", Amount: -40},
		// Another hour, merged in 3-hour columns.
		{At: at(22, 0), CategoryID: "fun", Amount: 400},
	}
	hm := NewSpendingHeatmap(Currency{String: "EGP"}, HeatmapByAmount, 1, time.Time{}, time.Time{}, time.Saturday, nil, txs, false)
	hm.SetCategories([]Category{
		{ID: "food", Name: "Food", Color: "red"},
		{ID: "fun", Name: "Fun", Color: "blue"},
		{ID: "cab", Name: "Cabs", Color: "amber"},
		{ID: "gone", Name: "Gone", Color: "lime"},
	})

	top, more := hm.TopCategories(hm.Cells[time.Thursday][21], 3)
	names := func(s []HeatCategoryShare) (out []string) {
		for _, c := range s {
			out = append(out, c.Name)
		}
		return out
	}
	if got := names(top); len(got) != 3 || got[0] != "Food" || got[1] != "Fun" || got[2] != "Cabs" || more != 1 {
		t.Errorf("by amount = %v +%d, want Food, Fun, Cabs +1 (Uncategorized)", got, more)
	}
	if top[0].Amount != 400 || top[0].Count != 2 {
		t.Errorf("Food = %+v, want 400 over 2", top[0].HeatSpend)
	}

	hm.Metric = HeatmapByCount
	top, _ = hm.TopCategories(hm.Cells[time.Thursday][21], 3)
	if got := names(top); got[0] != "Cabs" || got[1] != "Food" {
		t.Errorf("by count = %v, want Cabs (3) then Food (2)", got)
	}

	hm.Metric = HeatmapByAmount
	col := HeatColumn{From: 21, To: 24}
	top, _ = hm.TopCategories(hm.Cell(time.Thursday, col), 1)
	if len(top) != 1 || top[0].Name != "Fun" || top[0].Amount != 650 {
		t.Errorf("21–24 top = %+v, want Fun at 650", top)
	}
	if hm.Cells[time.Thursday][21].Categories["fun"].Amount != 250 {
		t.Error("merging a column changed the hour's cell")
	}
	all, _ := hm.TopCategories(hm.Cell(time.Thursday, col), 10)
	for _, c := range all {
		if c.Name == "Gone" {
			t.Error("a refund-only category listed")
		}
		if c.Name == "Uncategorized" && c.Color != "gray" {
			t.Errorf("uncategorized color = %q", c.Color)
		}
	}
}

// TestSpendingHeatmapRefundedCharges checks charges refunded in full (card
// holds released) net to nothing without the refunds counted as
// transactions, and leave the cell uncolored by amount.
func TestSpendingHeatmapRefundedCharges(t *testing.T) {
	eet := time.FixedZone("", 2*3600)
	at := time.Date(2026, 11, 18, 10, 13, 0, 0, eet) // a Wednesday
	var txs []SpendingTime
	for _, amount := range []float64{233.3, 5} {
		txs = append(txs,
			SpendingTime{At: at, CategoryID: "cab", Amount: amount},
			SpendingTime{At: at, CategoryID: "cab", Amount: -amount},
		)
	}
	hm := NewSpendingHeatmap(Currency{String: "EGP"}, HeatmapByAmount, 1, time.Time{}, time.Time{}, time.Saturday, nil, txs, false)

	c := hm.Cells[time.Wednesday][10]
	if c.Count != 2 || c.Amount != 0 || c.Refunded != 238.3 {
		t.Errorf("cell = %+v, want 2 charges, 0 net, 238.3 refunded", c.HeatSpend)
	}
	if hm.Total != 2 || hm.Timed != 2 {
		t.Errorf("total/timed = %d/%d, want the 2 charges", hm.Total, hm.Timed)
	}
	if l := hm.Level(c, 100); l != 0 {
		t.Errorf("level by amount = %d, want 0", l)
	}
	if top, more := hm.TopCategories(c, 3); len(top) != 0 || more != 0 {
		t.Errorf("categories = %+v, want none (netting to nothing)", top)
	}
	if cat := c.Categories["cab"]; cat.Count != 2 || cat.Refunded != 238.3 {
		t.Errorf("category = %+v", cat)
	}
}
