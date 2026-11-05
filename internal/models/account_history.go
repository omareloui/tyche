package models

import "time"

// AccountHistoryMonths is how many months back an account's details chart
// its balance, the current one included.
const AccountHistoryMonths = 6

// AccountHistoryFrom is the first day an account's details chart: the start
// of the month (starting on s) AccountHistoryMonths-1 months before today's.
func AccountHistoryFrom(s MonthStart, today time.Time) time.Time {
	return s.For(s.Of(today).Month.AddDate(0, -(AccountHistoryMonths - 1), 0)).From
}

// GamyaaHistoryRange is the days a gam'yaa's details chart: from the first
// of the month it starts in to its last installment or payout.
func GamyaaHistoryRange(g *Gamyaa) (from, to time.Time) {
	from = MonthOf(g.StartsOn)
	to = g.EndsOn()
	for _, t := range g.Turns {
		if d := g.MonthDate(t); d.After(to) {
			to = d
		}
	}
	return from, to
}

// AccountHistory is an account's balance day by day from From to Today,
// worked back from its current balance and what its fulfilled transactions
// moved in and out of it (by their dates, in its own currency, transfers and
// debts included, so not an analysis), then projected to To through its
// scheduled ones.
type AccountHistory struct {
	From, Today time.Time
	// To is the last day, Today without a projection.
	To time.Time
	// Start is the day the user's months start on.
	Start MonthStart
	// Days are the days money moved, oldest first.
	Days []DayFlow
	// Scheduled are the days money is scheduled to move, oldest first.
	Scheduled []DayFlow
	// Balance is the balance at the end of each day from From to To.
	Balance []float64
}

// NewAccountHistory works the balance back from balance (today's, after
// every fulfilled transaction) through days, the money moved per day since
// from, then ahead to to (Today when before it) through scheduled. Days after
// today (e.g. another time zone's today) count as today's, scheduled ones
// not after today (overdue) as tomorrow's.
func NewAccountHistory(s MonthStart, balance float64, days, scheduled []DayFlow, from, today, to time.Time) *AccountHistory {
	from, today, to = dateOnly(from), dateOnly(today), dateOnly(to)
	if to.Before(today) {
		to = today
	}
	n := max(daysFrom(from, today)+1, 1)
	total := n + daysFrom(today, to)
	net := make([]float64, total)
	for _, d := range days {
		i := daysFrom(from, d.Day)
		if i < 0 {
			continue
		}
		net[min(i, n-1)] += d.In - d.Out
	}
	if total > n {
		for _, d := range scheduled {
			i := max(daysFrom(from, d.Day), n)
			if i >= total {
				continue
			}
			net[i] += d.In - d.Out
		}
	}

	bal := make([]float64, total)
	bal[n-1] = balance
	for i := n - 1; i > 0; i-- {
		bal[i-1] = bal[i] - net[i]
	}
	for i := n; i < total; i++ {
		bal[i] = bal[i-1] + net[i]
	}
	return &AccountHistory{From: from, Today: today, To: to, Start: s, Days: days, Scheduled: scheduled, Balance: bal}
}

func daysFrom(from, t time.Time) int {
	return int(dateOnly(t).Sub(dateOnly(from)).Hours() / 24)
}

// DayOf is the date of the i-th balance.
func (h *AccountHistory) DayOf(i int) time.Time {
	return h.From.AddDate(0, 0, i)
}

// Index is the index of day's balance (out of range when not in it).
func (h *AccountHistory) Index(day time.Time) int {
	return daysFrom(h.From, day)
}

// TodayIdx is the index of today's balance.
func (h *AccountHistory) TodayIdx() int {
	return max(daysFrom(h.From, h.Today), 0)
}

// IsProjected tells if the i-th balance is after today, from scheduled
// transactions.
func (h *AccountHistory) IsProjected(i int) bool {
	return i > h.TodayIdx()
}

// HasProjection tells if it goes past today.
func (h *AccountHistory) HasProjection() bool {
	return len(h.Balance) > h.TodayIdx()+1
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

// ThisMonth is the money moved in and out in Today's month (days after
// Today counted, as in the balance).
func (h *AccountHistory) ThisMonth() (in, out float64) {
	p := h.Start.Of(h.Today)
	return h.InOut(p.From, p.To)
}

// MonthlyAvg is the money moved in and out a month on average over the
// full months before Today's.
func (h *AccountHistory) MonthlyAvg() (in, out float64) {
	cur := h.Start.Of(h.Today)
	months := monthsBetween(h.Start.Of(h.From).Month, cur.Month)
	if months <= 0 {
		return 0, 0
	}
	in, out = h.InOut(h.From, cur.From)
	return in / float64(months), out / float64(months)
}

// AccountTxStats is how many fulfilled transactions touched an account, and
// the days of the first and last (zero without any).
type AccountTxStats struct {
	Count       int
	First, Last time.Time
}
