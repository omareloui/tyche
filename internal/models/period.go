package models

import "time"

// MonthStart is the day of the month the user's months (budgets, analytics,
// the transactions list's months) start on, 1–31; on a shorter month a late
// day falls on its last day. A month is named after the one it ends in: with
// 25, November is Oct 25 – Nov 24.
type MonthStart int

// MaxMonthStart is the latest day a month can start on.
const MaxMonthStart = 31

// Day is the start day, 1 when unset or out of range.
func (s MonthStart) Day() int {
	if s < 1 || s > MaxMonthStart {
		return 1
	}
	return int(s)
}

// Period is one of the user's months.
type Period struct {
	// Month is the 1st of the month it's named after, in UTC.
	Month time.Time
	// From and To are its first day and the day after its last, in UTC.
	From, To time.Time
	Start    MonthStart
}

// For is the period named after month's month.
func (s MonthStart) For(month time.Time) Period {
	m := MonthOf(month)
	p := Period{Month: m, Start: s}
	d := s.Day()
	if d == 1 {
		p.From, p.To = m, m.AddDate(0, 1, 0)
		return p
	}
	p.From, p.To = startDay(m.AddDate(0, -1, 0), d), startDay(m, d)
	return p
}

// Plus is the period k months after the one t's day is in.
func (s MonthStart) Plus(t time.Time, k int) Period {
	return s.For(s.Of(t).Month.AddDate(0, k, 0))
}

// Of is the period t's day is in.
func (s MonthStart) Of(t time.Time) Period {
	day := dateOnly(t)
	p := s.For(day)
	if !day.Before(p.To) {
		return p.Next()
	}
	return p
}

// startDay is day d of month's month, or its last day if shorter.
func startDay(month time.Time, d int) time.Time {
	return time.Date(month.Year(), month.Month(), min(d, daysIn(month)), 0, 0, 0, 0, time.UTC)
}

func (p Period) Prev() Period {
	return p.Start.For(p.Month.AddDate(0, -1, 0))
}

func (p Period) Next() Period {
	return p.Start.For(p.Month.AddDate(0, 1, 0))
}

// Days counts its days.
func (p Period) Days() int {
	return p.Day(p.To)
}

// Day is the 0-based index of t's day in the period.
func (p Period) Day(t time.Time) int {
	return int(dateOnly(t).Sub(p.From).Hours() / 24)
}

func (p Period) Contains(t time.Time) bool {
	day := dateOnly(t)
	return !day.Before(p.From) && day.Before(p.To)
}

// Last is its last day.
func (p Period) Last() time.Time {
	return p.To.AddDate(0, 0, -1)
}

// IsCalendar tells whether it's a calendar month.
func (p Period) IsCalendar() bool {
	return p.From.Equal(p.Month)
}

// Key is the YYYY-MM of the month it's named after.
func (p Period) Key() string {
	return p.Month.Format("2006-01")
}

// RangeLabel is its days, e.g. "Oct 25 – Nov 24".
func (p Period) RangeLabel() string {
	return p.From.Format("Jan 2") + " – " + p.Last().Format("Jan 2")
}
