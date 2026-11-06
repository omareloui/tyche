package models_test

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

func TestSafeToSpend(t *testing.T) {
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	ctx := func(start int, today time.Time) models.AffordContext {
		return models.AffordContext{
			Balances:   []models.Balance{{Amount: 3000, Currency: egp}},
			BudgetLeft: 1000, Start: models.MonthStart(start), Today: today, Currency: &egp,
		}
	}
	bills := []models.Balance{{Amount: 500, Currency: egp}}

	tests := []struct {
		name     string
		start    int
		today    time.Time
		bills    []models.Balance
		daysLeft int
		free     float64
		perDay   float64
	}{
		{"first day", 1, day(2026, time.November, 1), bills, 30, 1500, 50},
		{"last day", 1, day(2026, time.November, 30), bills, 1, 1500, 1500},
		{"no bills", 1, day(2026, time.November, 21), nil, 10, 2000, 200},
		// November = Oct 25 – Nov 24, so 4 days left on the 21st.
		{"start 25", 25, day(2026, time.November, 21), bills, 4, 1500, 375},
		// December = Nov 25 – Dec 24.
		{"start 25 next month", 25, day(2026, time.November, 25), bills, 30, 1500, 50},
		{"over", 1, day(2026, time.November, 21), []models.Balance{{Amount: 2500, Currency: egp}}, 10, -500, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := models.NewSafeToSpend(ctx(tt.start, tt.today), tt.bills)
			if got := s.DaysLeft(); got != tt.daysLeft {
				t.Errorf("DaysLeft() = %d, want %d", got, tt.daysLeft)
			}
			if got := s.Free(); got != tt.free {
				t.Errorf("Free() = %v, want %v", got, tt.free)
			}
			if got := s.PerDay(); got != tt.perDay {
				t.Errorf("PerDay() = %v, want %v", got, tt.perDay)
			}
		})
	}
}
