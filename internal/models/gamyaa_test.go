package models_test

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

func TestGamyaaAmounts(t *testing.T) {
	tests := []struct {
		name                    string
		shares                  float64
		turns                   []int
		wantMonthly, wantPayout float64
	}{
		{name: "one share", shares: 1, turns: []int{4}, wantMonthly: 1000, wantPayout: 10000},
		{name: "half share", shares: 0.5, turns: []int{4}, wantMonthly: 500, wantPayout: 5000},
		{name: "two shares, two turns", shares: 2, turns: []int{2, 9}, wantMonthly: 2000, wantPayout: 10000},
		{name: "two shares, one turn", shares: 2, turns: []int{5}, wantMonthly: 2000, wantPayout: 20000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &models.Gamyaa{Installment: 1000, Members: 10, Shares: tt.shares, Turns: tt.turns}
			if got := g.MonthlyPayment(); got != tt.wantMonthly {
				t.Errorf("MonthlyPayment() = %v, want %v", got, tt.wantMonthly)
			}
			if got := g.PayoutAmount(); got != tt.wantPayout {
				t.Errorf("PayoutAmount() = %v, want %v", got, tt.wantPayout)
			}
			if got, want := g.PayoutAmount()*float64(len(tt.turns)), g.MonthlyPayment()*10; got != want {
				t.Errorf("payouts total %v, want what's paid in %v", got, want)
			}
		})
	}
}

func TestGamyaaMonthDate(t *testing.T) {
	g := &models.Gamyaa{StartsOn: time.Date(2027, time.January, 31, 0, 0, 0, 0, time.UTC), Members: 4}
	want := []string{"2027-01-31", "2027-02-28", "2027-03-31", "2027-04-30"}
	for i, w := range want {
		if got := g.MonthDate(i + 1).Format(time.DateOnly); got != w {
			t.Errorf("MonthDate(%d) = %s, want %s", i+1, got, w)
		}
	}
	if got := g.EndsOn().Format(time.DateOnly); got != want[3] {
		t.Errorf("EndsOn() = %s, want %s", got, want[3])
	}
}

func TestGamyaaSchedule(t *testing.T) {
	acc := &models.Account{ID: "gamyaa", Name: "Office"}
	g := &models.Gamyaa{
		Installment: 1000, Members: 3, Shares: 1, Turns: []int{2},
		StartsOn: time.Date(2027, time.January, 10, 0, 0, 0, 0, time.UTC),
		PayFrom:  &models.Account{ID: "bank"},
	}
	ins := g.Installments(acc, nil, time.Now())
	if len(ins) != 3 {
		t.Fatalf("got %d installments, want 3", len(ins))
	}
	for _, tx := range ins {
		if tx.Kind != models.TransferTxKind || tx.Account.ID != "bank" || tx.ToAccount.ID != "gamyaa" || tx.Amount != 1000 {
			t.Errorf("bad installment %+v", tx)
		}
	}
	pos := g.Payouts(acc, time.Now())
	if len(pos) != 1 {
		t.Fatalf("got %d payouts, want 1", len(pos))
	}
	if p := pos[0]; p.Account.ID != "gamyaa" || p.ToAccount.ID != "bank" || p.Amount != 3000 || p.ScheduledTo.Month() != time.February {
		t.Errorf("bad payout %+v", p)
	}
}
