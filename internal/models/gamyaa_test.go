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

func TestGamyaaMonths(t *testing.T) {
	day := func(s string) time.Time {
		d, _ := time.Parse(time.DateOnly, s)
		return d
	}
	g := &models.Gamyaa{Installment: 5000, Members: 10, Shares: 2, Turns: []int{7, 8}, StartsOn: day("2026-04-23")}

	for _, tt := range []struct {
		on   string
		want int
	}{
		{"2026-04-22", 0},
		{"2026-04-23", 1},
		{"2026-05-22", 1},
		{"2026-11-05", 7},
		{"2027-01-23", 10},
		{"2027-02-23", 11},
	} {
		if got := g.MonthOf(day(tt.on)); got != tt.want {
			t.Errorf("MonthOf(%s) = %d, want %d", tt.on, got, tt.want)
		}
	}
	if got := g.MonthsLeft(day("2026-11-05")); got != 3 {
		t.Errorf("MonthsLeft = %d, want 3", got)
	}

	g.AddMonthTx(day("2026-04-26"), 10000, false, true) // paid late in month 1
	g.AddMonthTx(day("2026-10-25"), 40000, true, true)  // payout received
	g.AddMonthTx(day("2026-11-23"), 10000, false, false)
	g.AddMonthTx(day("2026-11-23"), 50000, true, false)
	if len(g.Months) != 10 || g.Months[0].Paid != 10000 || g.Months[6].Received != 40000 || !g.Months[6].Turn {
		t.Errorf("months = %+v", g.Months)
	}
	m := g.Months[7]
	if m.Due != 10000 || m.Payout != 50000 || m.IsOverdue(day("2026-11-05")) || !m.IsOverdue(day("2026-11-24")) {
		t.Errorf("month 8 = %+v", m)
	}
}
