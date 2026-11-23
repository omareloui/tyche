package models

import (
	"fmt"
	"math"
	"slices"
	"time"
)

// HeatmapRanges are the numbers of user months the spending heatmap offers,
// the current one being 1.
var HeatmapRanges = []int{1, 3, 6, 12}

// HeatmapDefaultMonths is the heatmap's range when none is picked.
const HeatmapDefaultMonths = 3

// HeatmapMinTimed is how many transactions with a time of day the heatmap
// needs to show the hours, else only the weekdays.
const HeatmapMinTimed = 10

// HeatmapLevels is how many colors the heatmap's cells take, empty included.
const HeatmapLevels = 5

// heatmapNightEnd is the hour up to which empty night hours are left out of
// the grid.
const heatmapNightEnd = 7

// HeatmapMetric is what the heatmap colors by.
type HeatmapMetric string

const (
	HeatmapByAmount HeatmapMetric = "amount"
	HeatmapByCount  HeatmapMetric = "count"
)

// ParseHeatmapMetric is v's metric, by amount when unknown.
func ParseHeatmapMetric(v string) HeatmapMetric {
	if HeatmapMetric(v) == HeatmapByCount {
		return HeatmapByCount
	}
	return HeatmapByAmount
}

// ParseHeatmapMonths is v's months when one of HeatmapRanges, else
// HeatmapDefaultMonths.
func ParseHeatmapMonths(v string) int {
	for _, n := range HeatmapRanges {
		if v == fmt.Sprint(n) {
			return n
		}
	}
	return HeatmapDefaultMonths
}

// HeatmapPeriod is the n user months up to now's (included), from the first
// day of the oldest to the day after the current one's last.
func HeatmapPeriod(s MonthStart, now time.Time, n int) (from, to time.Time) {
	return s.Plus(now, -(n - 1)).From, s.Of(now).To
}

// SpendingTime is one transaction's spending, in the heatmap's currency, at
// its time as stored.
type SpendingTime struct {
	At     time.Time
	Amount float64
}

// HeatCell is the spending of a weekday, an hour or both.
type HeatCell struct {
	Amount float64
	Count  int
}

func (c *HeatCell) add(amount float64) {
	c.Amount += amount
	c.Count++
}

func (c HeatCell) plus(o HeatCell) HeatCell {
	return HeatCell{Amount: c.Amount + o.Amount, Count: c.Count + o.Count}
}

// SpendingHeatmap is when money went out: per weekday × hour of the user's
// time zone.
type SpendingHeatmap struct {
	Currency Currency
	Metric   HeatmapMetric
	// Months is how many user months it covers, From to To (exclusive).
	Months   int
	From, To time.Time
	// WeekStart is the first weekday of the rows.
	WeekStart time.Weekday
	// Cells are the timed transactions per weekday (time.Weekday) and hour.
	Cells [7][24]HeatCell
	// Weekdays are every transaction per weekday, the untimed ones included.
	Weekdays [7]HeatCell
	// Hours are the timed transactions per hour.
	Hours [24]HeatCell
	// Timed of Total transactions have a time of day.
	Timed, Total int
	// Partial is when a currency had no rate, its spending left out.
	Partial bool
}

// SpendingLocalTime is t in loc for the heatmap, and whether it has a time
// of day. A day without one is stored at midnight (UTC for the ones picked
// in forms), kept on its stored day. A time stored in a zone (SMS read in
// the user's then) stays in it, so DST and zone changes don't move it; one
// stored in UTC (the server's clock) is moved to loc.
func SpendingLocalTime(t time.Time, loc *time.Location) (time.Time, bool) {
	h, m, s := t.Clock()
	if h == 0 && m == 0 && s == 0 && t.Nanosecond() == 0 {
		return t, false
	}
	if _, off := t.Zone(); off == 0 && loc != nil {
		return t.In(loc), true
	}
	return t, true
}

// NewSpendingHeatmap buckets txs (in cur) into weekdays and hours of loc.
func NewSpendingHeatmap(cur Currency, metric HeatmapMetric, months int, from, to time.Time, weekStart time.Weekday, loc *time.Location, txs []SpendingTime, partial bool) *SpendingHeatmap {
	hm := &SpendingHeatmap{
		Currency:  cur,
		Metric:    metric,
		Months:    months,
		From:      from,
		To:        to,
		WeekStart: weekStart,
		Partial:   partial,
	}
	for _, tx := range txs {
		at, timed := SpendingLocalTime(tx.At, loc)
		wd := at.Weekday()
		hm.Total++
		hm.Weekdays[wd].add(tx.Amount)
		if !timed {
			continue
		}
		hm.Timed++
		hm.Cells[wd][at.Hour()].add(tx.Amount)
		hm.Hours[at.Hour()].add(tx.Amount)
	}
	return hm
}

// ShowHours is whether enough transactions have a time to show the hours.
func (hm *SpendingHeatmap) ShowHours() bool {
	return hm.Timed >= HeatmapMinTimed
}

// Days are the weekdays in the week's order.
func (hm *SpendingHeatmap) Days() []time.Weekday {
	days := make([]time.Weekday, 7)
	for i := range days {
		days[i] = (hm.WeekStart + time.Weekday(i)) % 7
	}
	return days
}

// FirstHour is the first hour the grid shows: the empty night hours (up to
// heatmapNightEnd) are left out.
func (hm *SpendingHeatmap) FirstHour() int {
	for h := range heatmapNightEnd {
		if hm.Hours[h].Count > 0 {
			return h
		}
	}
	return heatmapNightEnd
}

// HeatColumn is a column of the grid: hours From to To (exclusive).
type HeatColumn struct {
	From, To int
}

// Label is the column's first hour, e.g. "09".
func (c HeatColumn) Label() string {
	return fmt.Sprintf("%02d", c.From)
}

// RangeLabel is the column's hours, e.g. "21:00" or "21–24".
func (c HeatColumn) RangeLabel() string {
	if c.To-c.From == 1 {
		return fmt.Sprintf("%02d:00", c.From)
	}
	return fmt.Sprintf("%02d–%02d", c.From, c.To)
}

// Columns are the grid's hours by step, from FirstHour for single hours,
// the whole day for wider ones.
func (hm *SpendingHeatmap) Columns(step int) []HeatColumn {
	first := 0
	if step == 1 {
		first = hm.FirstHour()
	}
	var cols []HeatColumn
	for h := first; h < 24; h += step {
		cols = append(cols, HeatColumn{From: h, To: min(h+step, 24)})
	}
	return cols
}

// Cell is a weekday's spending in a column.
func (hm *SpendingHeatmap) Cell(wd time.Weekday, col HeatColumn) HeatCell {
	var c HeatCell
	for h := col.From; h < col.To; h++ {
		c = c.plus(hm.Cells[wd][h])
	}
	return c
}

// Hour is a column's spending, every weekday.
func (hm *SpendingHeatmap) Hour(col HeatColumn) HeatCell {
	var c HeatCell
	for h := col.From; h < col.To; h++ {
		c = c.plus(hm.Hours[h])
	}
	return c
}

// Value is what the heatmap colors c by.
func (hm *SpendingHeatmap) Value(c HeatCell) float64 {
	if hm.Metric == HeatmapByCount {
		return float64(c.Count)
	}
	return c.Amount
}

// Max is the largest value of the cells by step, for the color scale.
func (hm *SpendingHeatmap) Max(step int) float64 {
	var m float64
	for _, col := range hm.Columns(step) {
		for wd := range 7 {
			m = max(m, hm.Value(hm.Cell(time.Weekday(wd), col)))
		}
	}
	return m
}

// Level is c's color, 0 (nothing spent) to HeatmapLevels-1, against the max
// value. Amounts are scaled by their square root so a few large ones don't
// wash out the rest.
func (hm *SpendingHeatmap) Level(c HeatCell, maxValue float64) int {
	v := hm.Value(c)
	if v <= 0 || maxValue <= 0 || c.Count == 0 {
		return 0
	}
	r := min(v/maxValue, 1)
	if hm.Metric == HeatmapByAmount {
		r = math.Sqrt(r)
	}
	return 1 + min(int(r*float64(HeatmapLevels-1)), HeatmapLevels-2)
}

// WeekdayMax is the largest weekday value, for the weekday bars.
func (hm *SpendingHeatmap) WeekdayMax() float64 {
	var m float64
	for _, c := range hm.Weekdays {
		m = max(m, hm.Value(c))
	}
	return m
}

// Pct is v's share of m, 0–100.
func (hm *SpendingHeatmap) Pct(v, m float64) float64 {
	if m <= 0 || v <= 0 {
		return 0
	}
	return min(v/m*100, 100)
}

// Busiest is the weekday and column with the most of the metric by step,
// ok false when nothing has a time.
func (hm *SpendingHeatmap) Busiest(step int) (time.Weekday, HeatColumn, bool) {
	var (
		best   float64
		bestWd time.Weekday
		bestC  HeatColumn
		found  bool
	)
	for _, col := range hm.Columns(step) {
		for _, wd := range hm.Days() {
			if v := hm.Value(hm.Cell(wd, col)); v > best {
				best, bestWd, bestC, found = v, wd, col, true
			}
		}
	}
	return bestWd, bestC, found
}

// BusiestDay is the weekday with the most of the metric, ok false when
// nothing was spent.
func (hm *SpendingHeatmap) BusiestDay() (time.Weekday, bool) {
	days := hm.Days()
	i := slices.IndexFunc(days, func(wd time.Weekday) bool {
		return hm.Value(hm.Weekdays[wd]) == hm.WeekdayMax() && hm.WeekdayMax() > 0
	})
	if i < 0 {
		return 0, false
	}
	return days[i], true
}
