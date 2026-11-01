package models

import (
	"testing"
	"time"
)

func TestMatchesTransferPair(t *testing.T) {
	at := time.Date(2026, 10, 18, 12, 13, 0, 0, time.UTC)
	egp := Currency{String: "EGP"}
	usd := Currency{String: "USD"}
	current := Account{ID: "current", Currency: egp}
	card := Account{ID: "card", Currency: egp}
	dollars := Account{ID: "dollars", Currency: usd}
	rate := func(from, to string) float64 {
		if from == "USD" && to == "EGP" {
			return 50
		}
		return 0
	}
	out := func(amount float64) Transaction {
		return Transaction{ID: "out", Kind: ExpenseTxKind, Account: current, Amount: amount, FulfilledAt: at}
	}
	in := func(amount float64, after time.Duration) Transaction {
		return Transaction{ID: "in", Kind: IncomeTxKind, Account: card, Amount: amount, FulfilledAt: at.Add(after)}
	}

	tests := []struct {
		name    string
		out, in Transaction
		want    bool
	}{
		{"same amount", out(24715.73), in(24715.73, 5*time.Second), true},
		{"in first", out(24715.73), in(24715.73, -time.Hour), true},
		{"small fee", out(1005), in(1000, time.Hour), true},
		{"fee within the minimum", out(100), in(92, time.Hour), true},
		{"fee too big", out(1000), in(900, time.Hour), false},
		{"in more than out", out(1000), in(1000.5, time.Hour), false},
		{"too far apart", out(1000), in(1000, 25*time.Hour), false},
		{"same account", out(1000), func() Transaction { tx := in(1000, 0); tx.Account = current; return tx }(), false},
		{"merchant", func() Transaction {
			tx := out(1000)
			tx.Counterparty = &Counterparty{Name: "Shop", IsMerchant: true}
			return tx
		}(), in(1000, 0), false},
		{"goal", func() Transaction { tx := out(1000); tx.Goal = &Goal{ID: "g"}; return tx }(), in(1000, 0), false},
		{"reimbursement", out(1000), func() Transaction { tx := in(1000, 0); tx.ReimbursesID = "x"; return tx }(), false},
		{"already paired", out(1000), func() Transaction { tx := in(1000, 0); tx.TransferPairID = "x"; return tx }(), false},
		{"scheduled", out(1000), func() Transaction { tx := in(1000, 0); tx.FulfilledAt = time.Time{}; return tx }(), false},
		{"other currency", out(5000), func() Transaction { tx := in(101, 0); tx.Account = dollars; return tx }(), true},
		{"other currency too far", out(5000), func() Transaction { tx := in(80, 0); tx.Account = dollars; return tx }(), false},
		{"wrong kinds", in(1000, 0), out(1000), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesTransferPair(&tt.out, &tt.in, rate); got != tt.want {
				t.Errorf("MatchesTransferPair() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPickTransferPair(t *testing.T) {
	at := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	egp := Currency{String: "EGP"}
	tx := &Transaction{ID: "in", Kind: IncomeTxKind, Account: Account{ID: "card", Currency: egp}, Amount: 1000, FulfilledAt: at}
	cand := func(id string, amount float64, before time.Duration) Transaction {
		return Transaction{ID: id, Kind: ExpenseTxKind, Account: Account{ID: "current", Currency: egp}, Amount: amount, FulfilledAt: at.Add(-before)}
	}
	noRate := func(string, string) float64 { return 0 }

	got := PickTransferPair(tx, []Transaction{
		cand("far", 1000, 10*time.Hour),
		cand("close", 1001, time.Minute),
		cand("big", 5000, 0),
	}, noRate)
	if got == nil || got.ID != "close" {
		t.Errorf("picked %v, want close", got)
	}

	if got := PickTransferPair(tx, []Transaction{cand("big", 5000, 0)}, noRate); got != nil {
		t.Errorf("picked %s, want none", got.ID)
	}
}

func TestTransferLegAnalysisAmount(t *testing.T) {
	tx := Transaction{Kind: ExpenseTxKind, Amount: 100, TransferPairID: "x"}
	if got := tx.AnalysisAmount(); got != 0 {
		t.Errorf("AnalysisAmount() = %v, want 0", got)
	}
}
