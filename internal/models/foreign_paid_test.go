package models

import "testing"

func TestPaidView(t *testing.T) {
	egp := Currency{String: "EGP", Symbol: "E£"}
	fee := Transaction{Account: Account{Currency: egp}, Amount: 298.68, FxFee: 8.69}
	if got := fee.PaidView(); got != "Paid "+egp.Format(289.99)+" + "+egp.Format(8.69)+" fee" {
		t.Errorf("fee only = %q", got)
	}
	foreign := Transaction{Account: Account{Currency: egp}, Amount: 341.49, Original: &ForeignAmount{Amount: 6.36, Currency: "USD"}}
	if got := foreign.PaidView(); got != "Paid "+foreign.Original.View() {
		t.Errorf("foreign = %q", got)
	}
	if got := (&Transaction{Account: Account{Currency: egp}, Amount: 5}).PaidView(); got != "" {
		t.Errorf("local = %q", got)
	}
}
