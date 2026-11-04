package models

import (
	"slices"
	"testing"
)

func TestNewAccountHistory(t *testing.T) {
	days := []DayFlow{
		{Day: day("2026-10-30"), In: 50},           // before from: ignored
		{Day: day("2026-11-01"), In: 100, Out: 30}, // +70
		{Day: day("2026-11-03"), Out: 20},          // -20, nothing on the 2nd
		{Day: day("2026-11-05"), In: 10},           // after today: today's
	}
	h := NewAccountHistory(1, 500, days, day("2026-11-01"), day("2026-11-04"))

	// End of day balances worked back from 500: the 4th got +10, the 3rd -20.
	if got := h.Balance; !slices.Equal(got, []float64{510, 510, 490, 500}) {
		t.Errorf("balance = %v", got)
	}
	if !h.DayOf(3).Equal(day("2026-11-04")) {
		t.Errorf("DayOf(3) = %v", h.DayOf(3))
	}
	if lo, hi := h.Range(); lo != 0 || hi != 510 {
		t.Errorf("range = %v, %v", lo, hi)
	}
	if in, out := h.ThisMonth(); in != 110 || out != 50 {
		t.Errorf("this month = %v, %v", in, out)
	}
}

func TestNewAccountHistoryEmpty(t *testing.T) {
	h := NewAccountHistory(1, -200, nil, day("2026-06-01"), day("2026-11-04"))
	if len(h.Balance) != 157 || h.Balance[0] != -200 || h.Balance[156] != -200 {
		t.Errorf("balance = %d points, %v … %v", len(h.Balance), h.Balance[0], h.Balance[len(h.Balance)-1])
	}
	if lo, hi := h.Range(); lo != -200 || hi != 0 {
		t.Errorf("range = %v, %v", lo, hi)
	}
}

func TestAccountHistoryMonthlyAvg(t *testing.T) {
	days := []DayFlow{
		{Day: day("2026-09-10"), In: 300, Out: 100},
		{Day: day("2026-10-10"), In: 100, Out: 300},
		{Day: day("2026-11-02"), In: 1000}, // this month: left out
	}
	h := NewAccountHistory(1, 0, days, day("2026-09-01"), day("2026-11-04"))
	if in, out := h.MonthlyAvg(); in != 200 || out != 200 {
		t.Errorf("monthly avg = %v, %v", in, out)
	}
	if !AccountHistoryFrom(1, day("2026-11-04")).Equal(day("2026-06-01")) {
		t.Errorf("from = %v", AccountHistoryFrom(1, day("2026-11-04")))
	}
}
