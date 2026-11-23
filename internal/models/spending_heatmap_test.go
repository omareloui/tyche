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

	if hm.Total != 4 || hm.Timed != 3 {
		t.Errorf("total/timed = %d/%d, want 4/3", hm.Total, hm.Timed)
	}
	if c := hm.Cells[time.Thursday][21]; c.Amount != 70 || c.Count != 2 {
		t.Errorf("Thu 21 = %+v, want 70 over 2", c)
	}
	if c := hm.Cells[time.Friday][2]; c.Amount != 40 || c.Count != 1 {
		t.Errorf("Fri 02 = %+v, want the UTC time moved to Cairo", c)
	}
	if c := hm.Cells[time.Thursday][0]; c.Count != 0 {
		t.Errorf("Thu 00 = %+v, want the midnight tx out of the hours", c)
	}
	if c := hm.Weekdays[time.Thursday]; c.Amount != 570 || c.Count != 3 {
		t.Errorf("Thu = %+v, want 570 over 3 (the midnight one included)", c)
	}
	if hm.Hours[21].Count != 2 || hm.Hours[2].Count != 1 {
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
	hm := &SpendingHeatmap{Metric: HeatmapByCount}
	hm.Cells[time.Monday][9] = HeatCell{Amount: 10, Count: 4}
	hm.Cells[time.Monday][10] = HeatCell{Amount: 10, Count: 1}
	hm.Hours[9] = HeatCell{Count: 4}
	hm.Hours[10] = HeatCell{Count: 1}

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
