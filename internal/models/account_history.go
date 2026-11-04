package models

import "time"

// AccountHistoryMonths is how many months back an account's details chart
// its balance, the current one included.
const AccountHistoryMonths = 6

// AccountHistoryFrom is the first day an account's details chart: the 1st of
// the month AccountHistoryMonths-1 months before today's.
func AccountHistoryFrom(today time.Time) time.Time {
	return time.Date(today.Year(), today.Month()-(AccountHistoryMonths-1), 1, 0, 0, 0, 0, time.UTC)
}

// AccountHistory is an account's balance day by day from From to Today,
// worked back from its current balance and what its fulfilled transactions
// moved in and out of it (in its own currency, transfers and debts included,
// so not an analysis).
type AccountHistory struct {
	From, Today time.Time
	// Days are the days money moved, oldest first.
	Days []DayFlow
	// Balance is the balance at the end of each day from From to Today.
	Balance []float64
}

// NewAccountHistory works the balance back from balance (today's, after
// every fulfilled transaction) through days, the money moved per day since
// from. Days after today (e.g. another time zone's today) count as today's.
func NewAccountHistory(balance float64, days []DayFlow, from, today time.Time) *AccountHistory {
	from, today = dateOnly(from), dateOnly(today)
	n := max(int(today.Sub(from).Hours()/24)+1, 1)
	net := make([]float64, n)
	for _, d := range days {
		i := int(dateOnly(d.Day).Sub(from).Hours() / 24)
		if i < 0 {
			continue
		}
		net[min(i, n-1)] += d.In - d.Out
	}

	bal := make([]float64, n)
	bal[n-1] = balance
	for i := n - 1; i > 0; i-- {
		bal[i-1] = bal[i] - net[i]
	}
	return &AccountHistory{From: from, Today: today, Days: days, Balance: bal}
}

// DayOf is the date of the i-th balance.
func (h *AccountHistory) DayOf(i int) time.Time {
	return h.From.AddDate(0, 0, i)
}

// Range is the lowest and highest balance, 0 included so the chart's axis
// shows where it crosses it.
func (h *AccountHistory) Range() (lo, hi float64) {
	for _, b := range h.Balance {
		lo, hi = min(lo, b), max(hi, b)
	}
	return lo, hi
}

// InOut is the money moved in and out on the days from from to to
// (exclusive).
func (h *AccountHistory) InOut(from, to time.Time) (in, out float64) {
	for _, d := range h.Days {
		if day := dateOnly(d.Day); !day.Before(from) && day.Before(to) {
			in += d.In
			out += d.Out
		}
	}
	return in, out
}

// ThisMonth is the money moved in and out since the 1st of Today's month
// (days after Today counted, as in the balance).
func (h *AccountHistory) ThisMonth() (in, out float64) {
	start := time.Date(h.Today.Year(), h.Today.Month(), 1, 0, 0, 0, 0, time.UTC)
	return h.InOut(start, start.AddDate(0, 1, 0))
}

// MonthlyAvg is the money moved in and out a month on average over the
// full months before Today's.
func (h *AccountHistory) MonthlyAvg() (in, out float64) {
	start := time.Date(h.Today.Year(), h.Today.Month(), 1, 0, 0, 0, 0, time.UTC)
	months := (start.Year()-h.From.Year())*12 + int(start.Month()-h.From.Month())
	if months <= 0 {
		return 0, 0
	}
	in, out = h.InOut(h.From, start)
	return in / float64(months), out / float64(months)
}

// AccountTxStats is how many fulfilled transactions touched an account, and
// the days of the first and last (zero without any).
type AccountTxStats struct {
	Count       int
	First, Last time.Time
}
