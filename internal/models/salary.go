package models

import "github.com/omareloui/tyche/internal/payroll"

// SalaryRule is a user's own tax rules for the salary calculator.
type SalaryRule struct {
	ID, Name string
	// Country is its ISO 3166 alpha-2 code.
	Country string
	Rules   payroll.Rules
}
