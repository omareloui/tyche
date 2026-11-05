package models

import "testing"

func TestBankEntry(t *testing.T) {
	for _, c := range []struct {
		name            string
		kind            AccKind
		amount, aside   float64
		entry, typed    float64
		wantEntry, want float64
	}{
		{name: "bank", kind: AccKindBankAcc, amount: 100, typed: 150, wantEntry: 100, want: 150},
		{name: "set aside for goals", kind: AccKindBankAcc, amount: 100, aside: 40, typed: 150, wantEntry: 140, want: 110},
		{name: "card owed", kind: AccKindCreditCard, amount: -100, typed: 250, wantEntry: 100, want: -250},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := &Account{ID: "x", Amount: c.amount, SetAside: c.aside, Kind: AccountKind{Enum: c.kind}}
			if got := a.BankEntry(); got != c.wantEntry {
				t.Errorf("BankEntry = %v, want %v", got, c.wantEntry)
			}
			if got := a.FixOfBankEntry(c.typed).Amount; got != c.want {
				t.Errorf("FixOfBankEntry(%v) = %v, want %v", c.typed, got, c.want)
			}
			if got := a.FixOfBankEntry(a.BankEntry()).Amount; got != c.amount {
				t.Errorf("FixOfBankEntry(BankEntry) = %v, want the balance %v", got, c.amount)
			}
		})
	}
}
