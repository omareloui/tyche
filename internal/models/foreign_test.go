package models

import "testing"

func TestForeignCharge(t *testing.T) {
	egp := Currency{String: "EGP"}
	tests := []struct {
		name      string
		pct, min  float64
		kind      TxKind
		amount    float64
		rate      float64
		wantTotal float64
		wantFee   float64
	}{
		{name: "no fee", kind: ExpenseTxKind, amount: 10, rate: 50.123, wantTotal: 501.23},
		{name: "percent", pct: 3, kind: ExpenseTxKind, amount: 100, rate: 50, wantTotal: 5150, wantFee: 150},
		{name: "minimum wins", pct: 3, min: 20, kind: ExpenseTxKind, amount: 10, rate: 50, wantTotal: 520, wantFee: 20},
		{name: "minimum alone", min: 15, kind: ExpenseTxKind, amount: 10, rate: 50, wantTotal: 515, wantFee: 15},
		{name: "fee rounded", pct: 2.5, kind: ExpenseTxKind, amount: 1, rate: 50.37, wantTotal: 51.63, wantFee: 1.26},
		{name: "income takes none", pct: 3, min: 20, kind: IncomeTxKind, amount: 10, rate: 50, wantTotal: 500},
		{name: "refund takes none", pct: 3, kind: RefundTxKind, amount: 10, rate: 50, wantTotal: 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acc := &Account{Currency: egp, FxFeePercent: tt.pct, FxFeeMin: tt.min}
			total, fee := ForeignCharge(tt.amount, tt.rate, acc, tt.kind)
			if total != tt.wantTotal || fee != tt.wantFee {
				t.Errorf("ForeignCharge = %v, %v, want %v, %v", total, fee, tt.wantTotal, tt.wantFee)
			}
		})
	}
}

func TestOriginalRate(t *testing.T) {
	tx := &Transaction{
		Account:  Account{Currency: Currency{String: "EGP"}},
		Kind:     ExpenseTxKind,
		Amount:   5150,
		FxFee:    150,
		Original: &ForeignAmount{Amount: 100, Currency: "USD"},
	}
	if got := tx.OriginalRate(); got != 50 {
		t.Errorf("OriginalRate = %v, want 50", got)
	}
	tx.Original.Currency = "egp"
	if tx.IsForeign() {
		t.Error("paid in the account's currency isn't foreign")
	}
}

func TestAccountViewFxFee(t *testing.T) {
	acc := &Account{Currency: Currency{Symbol: "EGP"}, FxFeePercent: 3, FxFeeMin: 20}
	if got := acc.ViewFxFee(); got != "3% · min EGP 20" {
		t.Errorf("ViewFxFee = %q", got)
	}
}
