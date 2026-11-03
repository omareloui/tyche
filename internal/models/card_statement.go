package models

import (
	"fmt"
	"math"
	"time"

	"github.com/omareloui/tyche/internal/errs"
)

// CardCycle is a credit card's statement cycle: the statement closes on
// StatementDay (clamped to the month's length, closing at the end of the day)
// and its balance is due DueDays after, or on DueDay of the following month.
// Exactly one of DueDays and DueDay is set with StatementDay.
type CardCycle struct {
	StatementDay int
	DueDays      int
	DueDay       int
}

// IsSet reports whether the card has a statement cycle.
func (c CardCycle) IsSet() bool { return c.StatementDay > 0 }

// Validate gives the errors of a cycle being set, keyed by StatementDay and
// DueValue (the form's days after / day of month input).
func (c CardCycle) Validate() errs.ValidationErrors {
	ve := errs.ValidationErrors{}
	if c.StatementDay == 0 && c.DueDays == 0 && c.DueDay == 0 {
		return ve
	}
	inRange := func(field string, v, lo, hi int) {
		switch {
		case v == 0:
			ve[field] = errs.ValidationField{Tag: "required"}
		case v < lo:
			ve[field] = errs.ValidationField{Tag: "gte", Param: fmt.Sprint(lo)}
		case v > hi:
			ve[field] = errs.ValidationField{Tag: "lte", Param: fmt.Sprint(hi)}
		}
	}
	inRange("StatementDay", c.StatementDay, 1, 31)
	if c.DueDay != 0 {
		inRange("DueValue", c.DueDay, 1, 31)
	} else {
		inRange("DueValue", c.DueDays, 1, 60)
	}
	return ve
}

// dayIn is day of the month, clamped to its length, at midnight.
func dayIn(year int, month time.Month, day int, loc *time.Location) time.Time {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
	return time.Date(year, month, min(day, last), 0, 0, 0, 0, loc)
}

func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// LastClose is the day the last statement closed, before today.
func (c CardCycle) LastClose(today time.Time) time.Time {
	today = midnight(today)
	d := dayIn(today.Year(), today.Month(), c.StatementDay, today.Location())
	if !d.Before(today) {
		d = dayIn(today.Year(), today.Month()-1, c.StatementDay, today.Location())
	}
	return d
}

// NextClose is the day the statement in progress closes, today or later.
func (c CardCycle) NextClose(today time.Time) time.Time {
	today = midnight(today)
	d := dayIn(today.Year(), today.Month(), c.StatementDay, today.Location())
	if d.Before(today) {
		d = dayIn(today.Year(), today.Month()+1, c.StatementDay, today.Location())
	}
	return d
}

// DueOn is when the statement closed on closed is due.
func (c CardCycle) DueOn(closed time.Time) time.Time {
	if c.DueDay > 0 {
		d := dayIn(closed.Year(), closed.Month(), c.DueDay, closed.Location())
		if !d.After(closed) {
			d = dayIn(closed.Year(), closed.Month()+1, c.DueDay, closed.Location())
		}
		return d
	}
	return closed.AddDate(0, 0, c.DueDays)
}

// PurchaseDueOn is when a purchase made today has to be paid by, to be free
// of interest: the due date of the statement in progress.
func (c CardCycle) PurchaseDueOn(today time.Time) time.Time {
	return c.DueOn(c.NextClose(today))
}

// InterestFreeDays is how long a purchase made today stays free of interest.
func (c CardCycle) InterestFreeDays(today time.Time) int {
	return daysBetween(midnight(today), c.PurchaseDueOn(today))
}

func daysBetween(from, to time.Time) int {
	return int(math.Round(to.Sub(from).Hours() / 24))
}

// CardStatementStatus is how a statement's payment stands.
type CardStatementStatus uint8

const (
	// StatementPaid is a statement with nothing left to pay.
	StatementPaid CardStatementStatus = iota
	// StatementDue is due in more than StatementSoonDays.
	StatementDue
	// StatementDueSoon is due within StatementSoonDays.
	StatementDueSoon
	// StatementOverdue is past its due date and not paid in full.
	StatementOverdue
)

// StatementSoonDays is how close a due date has to be to show it urgent.
const StatementSoonDays = 3

// CardStatement is a credit card's last closed statement.
type CardStatement struct {
	Cycle    CardCycle
	ClosedOn time.Time
	DueOn    time.Time
	// Balance is what the card owed when the statement closed (positive).
	Balance float64
	// Paid is what was paid into the card since.
	Paid float64
	// Due is what's left to pay by DueOn to avoid interest.
	Due float64
	// SoFar is what the card owes now, the last statement's Due included.
	SoFar float64
	// Scheduled is what the unfulfilled transactions due by the next close
	// add to what it owes (negative for scheduled payments).
	Scheduled float64
	// Projected is what the next statement will be: SoFar plus Scheduled.
	Projected float64
	Currency  Currency
	// Today is the day, in the user's time zone, it was worked out on.
	Today time.Time
}

// NewCardStatement works out acc's last statement and projects the next one
// from its balance and what its transactions did to it: net the signed total
// of the ones fulfilled since the last statement closed, paidIn only what they
// added, scheduled the signed total of the unfulfilled ones due by the next
// close. nil if acc has no cycle.
func NewCardStatement(acc *Account, today time.Time, net, paidIn, scheduled float64) *CardStatement {
	if !acc.Cycle.IsSet() {
		return nil
	}
	closed := acc.Cycle.LastClose(today)
	cur := acc.Currency
	// cards' balances are negative when they owe
	owed := cur.Round(max(0, -(acc.Amount - net)))
	soFar := cur.Round(max(0, -acc.Amount))
	return &CardStatement{
		Cycle:     acc.Cycle,
		ClosedOn:  closed,
		DueOn:     acc.Cycle.DueOn(closed),
		Balance:   owed,
		Paid:      cur.Round(paidIn),
		Due:       cur.Round(max(0, owed-paidIn)),
		SoFar:     soFar,
		Scheduled: cur.Round(-scheduled),
		Projected: cur.Round(max(0, soFar-scheduled)),
		Currency:  cur,
		Today:     midnight(today),
	}
}

// Since is when the transactions since the statement closed start: the day
// after it.
func (s *CardStatement) Since() time.Time { return s.ClosedOn.AddDate(0, 0, 1) }

// DaysLeft is the days until the due date, negative once it's passed.
func (s *CardStatement) DaysLeft() int { return daysBetween(s.Today, s.DueOn) }

func (s *CardStatement) Status() CardStatementStatus {
	switch {
	case s.Due <= 0:
		return StatementPaid
	case s.DaysLeft() < 0:
		return StatementOverdue
	case s.DaysLeft() <= StatementSoonDays:
		return StatementDueSoon
	default:
		return StatementDue
	}
}

func (s *CardStatement) ViewDue() string { return s.Currency.Format(s.Due) }

func (s *CardStatement) ViewBalance() string { return s.Currency.Format(s.Balance) }

// NextClose is when the statement in progress closes.
func (s *CardStatement) NextClose() time.Time { return s.Cycle.NextClose(s.Today) }

// DaysToClose is the days until the next statement closes.
func (s *CardStatement) DaysToClose() int { return daysBetween(s.Today, s.NextClose()) }

// PurchaseDueOn is when a purchase made today is due.
func (s *CardStatement) PurchaseDueOn() time.Time { return s.Cycle.PurchaseDueOn(s.Today) }

// InterestFreeDays is how long a purchase made today stays free of interest.
func (s *CardStatement) InterestFreeDays() int { return s.Cycle.InterestFreeDays(s.Today) }

func (s *CardStatement) ViewSoFar() string { return s.Currency.Format(s.SoFar) }

func (s *CardStatement) ViewProjected() string { return s.Currency.Format(s.Projected) }

// ViewScheduled is Scheduled signed, e.g. "+ EGP 200".
func (s *CardStatement) ViewScheduled() string {
	if s.Scheduled < 0 {
		return "− " + s.Currency.Format(-s.Scheduled)
	}
	return "+ " + s.Currency.Format(s.Scheduled)
}

func (s *CardStatement) HasScheduled() bool { return s.Scheduled != 0 }

// Carried is what's left to pay of the last statement, in SoFar until paid.
func (s *CardStatement) Carried() float64 { return s.Due }

func (s *CardStatement) ViewCarried() string { return s.Currency.Format(s.Carried()) }
