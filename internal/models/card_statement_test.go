package models

import (
	"testing"
	"time"
)

func ymd(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestCardCycleCloses(t *testing.T) {
	tests := []struct {
		name  string
		stmt  int
		today time.Time
		last  time.Time
		next  time.Time
	}{
		{"before the day", 25, ymd(2026, 11, 3), ymd(2026, 10, 25), ymd(2026, 11, 25)},
		{"after the day", 25, ymd(2026, 11, 27), ymd(2026, 11, 25), ymd(2026, 12, 25)},
		{"on the day it's still open", 25, ymd(2026, 11, 25), ymd(2026, 10, 25), ymd(2026, 11, 25)},
		{"31st clamped in feb", 31, ymd(2026, 2, 10), ymd(2026, 1, 31), ymd(2026, 2, 28)},
		{"31st clamped in a leap feb", 31, ymd(2028, 3, 1), ymd(2028, 2, 29), ymd(2028, 3, 31)},
		{"across the year", 5, ymd(2027, 1, 2), ymd(2026, 12, 5), ymd(2027, 1, 5)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := CardCycle{StatementDay: tt.stmt, DueDays: 20}
			if got := c.LastClose(tt.today.Add(15 * time.Hour)); !got.Equal(tt.last) {
				t.Errorf("LastClose = %v, want %v", got, tt.last)
			}
			if got := c.NextClose(tt.today); !got.Equal(tt.next) {
				t.Errorf("NextClose = %v, want %v", got, tt.next)
			}
		})
	}
}

func TestCardCycleDueOn(t *testing.T) {
	tests := []struct {
		name  string
		c     CardCycle
		close time.Time
		want  time.Time
	}{
		{"days after", CardCycle{StatementDay: 25, DueDays: 25}, ymd(2026, 11, 25), ymd(2026, 12, 20)},
		{"day of next month", CardCycle{StatementDay: 25, DueDay: 15}, ymd(2026, 11, 25), ymd(2026, 12, 15)},
		{"day later the same month", CardCycle{StatementDay: 1, DueDay: 20}, ymd(2026, 11, 1), ymd(2026, 11, 20)},
		{"day clamped", CardCycle{StatementDay: 5, DueDay: 31}, ymd(2027, 1, 31), ymd(2027, 2, 28)},
		{"same day goes to next month", CardCycle{StatementDay: 10, DueDay: 10}, ymd(2026, 11, 10), ymd(2026, 12, 10)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.DueOn(tt.close); !got.Equal(tt.want) {
				t.Errorf("DueOn = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCardCycleInterestFree(t *testing.T) {
	c := CardCycle{StatementDay: 25, DueDays: 25}
	today := ymd(2026, 11, 26)
	if got, want := c.PurchaseDueOn(today), ymd(2027, 1, 19); !got.Equal(want) {
		t.Errorf("PurchaseDueOn = %v, want %v", got, want)
	}
	if got := c.InterestFreeDays(today); got != 54 {
		t.Errorf("InterestFreeDays = %d, want 54", got)
	}
	if got := c.InterestFreeDays(ymd(2026, 11, 25)); got != 25 {
		t.Errorf("InterestFreeDays on the statement day = %d, want 25", got)
	}
}

func TestCardCycleValidate(t *testing.T) {
	tests := []struct {
		c    CardCycle
		want []string
	}{
		{CardCycle{}, nil},
		{CardCycle{StatementDay: 25, DueDays: 20}, nil},
		{CardCycle{StatementDay: 25, DueDay: 15}, nil},
		{CardCycle{StatementDay: 32, DueDays: 20}, []string{"StatementDay"}},
		{CardCycle{StatementDay: 25}, []string{"DueValue"}},
		{CardCycle{StatementDay: 25, DueDays: 61}, []string{"DueValue"}},
		{CardCycle{StatementDay: 25, DueDay: 40}, []string{"DueValue"}},
		{CardCycle{DueDays: 20}, []string{"StatementDay"}},
	}
	for _, tt := range tests {
		ve := tt.c.Validate()
		if len(ve) != len(tt.want) {
			t.Errorf("%+v: errors %v, want %v", tt.c, ve, tt.want)
			continue
		}
		for _, f := range tt.want {
			if _, ok := ve[f]; !ok {
				t.Errorf("%+v: no %s error in %v", tt.c, f, ve)
			}
		}
	}
}

func TestNewCardStatement(t *testing.T) {
	cur := Currency{String: "EGP"}
	acc := &Account{Amount: -3000, Currency: cur, Cycle: CardCycle{StatementDay: 25, DueDays: 25}}
	today := ymd(2026, 12, 5)

	// since the close: spent 1000, paid in 2500 → net +1500
	s := NewCardStatement(acc, today, 1500, 2500, 0)
	if !s.ClosedOn.Equal(ymd(2026, 11, 25)) || !s.DueOn.Equal(ymd(2026, 12, 20)) {
		t.Fatalf("closed %v due %v", s.ClosedOn, s.DueOn)
	}
	if s.Balance != 4500 || s.Due != 2000 {
		t.Errorf("balance %v due %v, want 4500 / 2000", s.Balance, s.Due)
	}
	if s.Status() != StatementDue || s.DaysLeft() != 15 {
		t.Errorf("status %v days left %d", s.Status(), s.DaysLeft())
	}

	if s := NewCardStatement(acc, ymd(2026, 12, 18), 1500, 2500, 0); s.Status() != StatementDueSoon {
		t.Errorf("status %v, want soon", s.Status())
	}
	if s := NewCardStatement(acc, ymd(2026, 12, 21), 1500, 2500, 0); s.Status() != StatementOverdue {
		t.Errorf("status %v, want overdue", s.Status())
	}
	// paid more than the statement (spent 5000 since, paid in 6000)
	if s := NewCardStatement(acc, today, 1000, 6000, 0); s.Due != 0 || s.Status() != StatementPaid {
		t.Errorf("due %v status %v, want paid", s.Due, s.Status())
	}
	if NewCardStatement(&Account{}, today, 0, 0, 0) != nil {
		t.Error("statement of a card without a cycle")
	}
}

func TestCardStatementProjection(t *testing.T) {
	cur := Currency{String: "EGP"}
	acc := &Account{Amount: -1100, Currency: cur, Cycle: CardCycle{StatementDay: 25, DueDays: 25}}
	today := ymd(2026, 12, 5)

	tests := []struct {
		name      string
		scheduled float64
		want      float64
		wantSched float64
	}{
		{"nothing scheduled", 0, 1100, 0},
		{"a scheduled expense", -200, 1300, 200},
		{"a scheduled payment", 500, 600, -500},
		{"paid off by a scheduled payment", 2000, 0, -2000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewCardStatement(acc, today, 0, 0, tt.scheduled)
			if s.SoFar != 1100 || s.Projected != tt.want || s.Scheduled != tt.wantSched {
				t.Errorf("so far %v projected %v scheduled %v, want 1100 / %v / %v", s.SoFar, s.Projected, s.Scheduled, tt.want, tt.wantSched)
			}
		})
	}
}
