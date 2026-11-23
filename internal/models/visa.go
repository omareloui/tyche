package models

import (
	"github.com/omareloui/tyche/internal/visa"
)

// VisaRule is a user's own visa requirements for the visa readiness tool.
type VisaRule struct {
	ID, Name string
	// Country is its ISO 3166 alpha-2 code.
	Country string
	Rule    visa.Rule
}

// VisaMonths is how many full months of income the visa readiness tool
// reads.
const VisaMonths = 12

// VisaMoney is the user's money the visa readiness tool checks, in the
// default currency.
type VisaMoney struct {
	// Months are the last VisaMonths full user months' income, oldest
	// first; Periods their months.
	Months  []float64
	Periods []Period
	// Savings are the balance accounts' money plus what the goals in
	// progress hold.
	Savings float64
	Partial bool
}

// HasIncome tells some month had income.
func (m *VisaMoney) HasIncome() bool {
	for _, v := range m.Months {
		if v > 0 {
			return true
		}
	}
	return false
}

// Average is the months' income a month.
func (m *VisaMoney) Average() float64 {
	if len(m.Months) == 0 {
		return 0
	}
	var sum float64
	for _, v := range m.Months {
		sum += v
	}
	return sum / float64(len(m.Months))
}
