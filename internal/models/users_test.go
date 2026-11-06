package models_test

import (
	"testing"

	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/models"
)

func TestSalaryIsLike(t *testing.T) {
	t.Cleanup(fxrate.SetRates(map[string]map[string]float64{"usd": {"egp": 50}}))
	egp, usd, eur := &models.Currency{String: "EGP"}, &models.Currency{String: "USD"}, &models.Currency{String: "EUR"}
	s := models.Salary{Amount: 50_000, Currency: "EGP"}
	// The smallest amount blurred, in EGP and in USD (at 50).
	least := s.Amount * models.SalaryLikeShare
	leastUSD := least / 50

	for _, tt := range []struct {
		name   string
		s      models.Salary
		amount float64
		cur    *models.Currency
		want   bool
	}{
		{"the salary", s, s.Amount, egp, true},
		{"the share of it", s, least, egp, true},
		{"under the share", s, least - 1, egp, false},
		{"a negative adjustment", s, -least, egp, true},
		{"a negative adjustment, under", s, -(least - 1), egp, false},
		{"converted", s, leastUSD + 0.01, usd, true},
		{"converted, under", s, leastUSD - 0.01, usd, false},
		{"no rate", s, 1_000_000, eur, false},
		{"no salary", models.Salary{}, 1_000_000, egp, false},
	} {
		if got := tt.s.IsLike(tt.amount, tt.cur); got != tt.want {
			t.Errorf("%s: IsLike(%v %s) = %v, want %v", tt.name, tt.amount, tt.cur.String, got, tt.want)
		}
	}
}
