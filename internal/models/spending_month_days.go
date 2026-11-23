package models

import (
	"time"
)

// MonthDaysRow is one user month's spending per day, its first day first.
type MonthDaysRow struct {
	Period Period
	Days   []HeatCell
}

// MonthDaysHeatmap is when money went out across the user's months: a row
// per month in range, a column per day since the month's start (payday
// lined up), and All summing the months per column.
type MonthDaysHeatmap struct {
	HeatScale
	Currency Currency
	// Months is how many user months it covers, From to To (exclusive).
	Months   int
	From, To time.Time
	// Today is the user's today (a UTC date): days after it are ahead.
	Today time.Time
	// Rows are the months, oldest first.
	Rows []MonthDaysRow
	// All is every month's spending per column.
	All []HeatCell
	// Total counts the charges (refunds aren't counted).
	Total int
	// Partial is when a currency had no rate, its spending left out.
	Partial bool
}

// NewMonthDaysHeatmap buckets txs (in cur) into the days of the user's
// months (starting on s) from from to to, a tx's day being its day in loc
// (SpendingLocalTime).
func NewMonthDaysHeatmap(s MonthStart, cur Currency, metric HeatmapMetric, months int, from, to, today time.Time, loc *time.Location, txs []SpendingTime, partial bool) *MonthDaysHeatmap {
	hm := &MonthDaysHeatmap{
		HeatScale: HeatScale{Metric: metric},
		Currency:  cur,
		Months:    months,
		From:      from,
		To:        to,
		Today:     dateOnly(today),
		Partial:   partial,
	}
	longest := 0
	for p := s.Of(from); p.From.Before(to); p = p.Next() {
		hm.Rows = append(hm.Rows, MonthDaysRow{Period: p, Days: make([]HeatCell, p.Days())})
		longest = max(longest, p.Days())
	}
	hm.All = make([]HeatCell, longest)

	for _, tx := range txs {
		at, _ := SpendingLocalTime(tx.At, loc)
		day := dateOnly(at)
		for i := range hm.Rows {
			row := &hm.Rows[i]
			if !row.Period.Contains(day) {
				continue
			}
			k := row.Period.Day(day)
			row.Days[k].add(tx.CategoryID, tx.Amount)
			hm.All[k].add(tx.CategoryID, tx.Amount)
			if tx.Amount >= 0 {
				hm.Total++
			}
			break
		}
	}
	return hm
}

// Columns counts the grid's columns: the longest month's days.
func (hm *MonthDaysHeatmap) Columns() int {
	return len(hm.All)
}

// longest is the month with the most days, the latest of them.
func (hm *MonthDaysHeatmap) longest() Period {
	var p Period
	for _, row := range hm.Rows {
		if row.Period.Days() >= p.Days() {
			p = row.Period
		}
	}
	return p
}

// ColumnLabel is column k's day of the month in the longest month, e.g. 26.
func (hm *MonthDaysHeatmap) ColumnLabel(k int) int {
	return hm.longest().From.AddDate(0, 0, k).Day()
}

// Has is whether the month of row has a column k.
func (hm *MonthDaysHeatmap) Has(row, k int) bool {
	return k < len(hm.Rows[row].Days)
}

// Date is the day of row's column k.
func (hm *MonthDaysHeatmap) Date(row, k int) time.Time {
	return hm.Rows[row].Period.From.AddDate(0, 0, k)
}

// IsAhead is whether row's column k is after today.
func (hm *MonthDaysHeatmap) IsAhead(row, k int) bool {
	return hm.Date(row, k).After(hm.Today)
}

// IsToday is whether row's column k is today.
func (hm *MonthDaysHeatmap) IsToday(row, k int) bool {
	return hm.Date(row, k).Equal(hm.Today)
}

// RowsMax is the largest value of the months' days, their color scale.
func (hm *MonthDaysHeatmap) RowsMax() float64 {
	var m float64
	for _, row := range hm.Rows {
		for _, c := range row.Days {
			m = max(m, hm.Value(c))
		}
	}
	return m
}

// AllMax is the largest value of All, its own color scale.
func (hm *MonthDaysHeatmap) AllMax() float64 {
	var m float64
	for _, c := range hm.All {
		m = max(m, hm.Value(c))
	}
	return m
}

// BusiestColumn is the column of All with the most of the metric, ok false
// when nothing was spent.
func (hm *MonthDaysHeatmap) BusiestColumn() (int, bool) {
	best, found := 0.0, -1
	for k, c := range hm.All {
		if v := hm.Value(c); v > best {
			best, found = v, k
		}
	}
	return found, found >= 0
}

// Peak is the day with the most of the metric, ok false when nothing was
// spent.
func (hm *MonthDaysHeatmap) Peak() (time.Time, bool) {
	var (
		best float64
		day  time.Time
	)
	for i, row := range hm.Rows {
		for k, c := range row.Days {
			if v := hm.Value(c); v > best {
				best, day = v, hm.Date(i, k)
			}
		}
	}
	return day, !day.IsZero()
}
