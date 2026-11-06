package models

import (
	"math"
	"time"
)

// SafeToSpend is what can be spent a day until the end of the user's
// month: the balance less what's already promised this month (the budgets
// left, the goal plans' deposits and payments due, the scheduled money no
// budget covers), over the days left. In the default currency.
type SafeToSpend struct {
	Balance    float64
	BudgetLeft float64
	GoalsDue   float64
	Bills      float64
	// Period is the user's month today is in.
	Period Period
	Today  time.Time
}

// NewSafeToSpend works it out from the afford context (balances, budgets
// left, goals, today) and the scheduled outflows no budget covers until
// the end of the month.
func NewSafeToSpend(c AffordContext, bills []Balance) SafeToSpend {
	bal, dueNow := c.spendableParts()
	s := SafeToSpend{
		Balance: bal, BudgetLeft: c.BudgetLeft, GoalsDue: dueNow,
		Period: c.Start.Of(c.Today), Today: dateOnly(c.Today),
	}
	for _, b := range bills {
		s.Bills += convert(b.Amount, &b.Currency, c.Currency)
	}
	return s
}

// Free is the money left once this month's promises are kept, negative
// when they take more than the balance.
func (s SafeToSpend) Free() float64 {
	return RoundCents(s.Balance - s.BudgetLeft - s.GoalsDue - s.Bills)
}

// DaysLeft counts the days left in the month, today included.
func (s SafeToSpend) DaysLeft() int {
	return max(1, s.Period.Days()-s.Period.Day(s.Today))
}

// PerDay is what can be spent each day left, 0 when nothing is free.
func (s SafeToSpend) PerDay() float64 {
	return RoundCents(math.Max(0, s.Free()) / float64(s.DaysLeft()))
}
