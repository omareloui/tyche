package models

import (
	"net/url"
	"strings"
	"time"
)

// ItemsRange is the days the items page's stats count: a preset (the
// user's months, see MonthStart) or a custom range. From and To bound the
// lines' transaction days, To exclusive; zero for no bound.
type ItemsRange struct {
	Key      string
	From, To time.Time
}

// The items page's ranges (its ?range=), every one by default.
const (
	ItemsRangeAll       = "all"
	ItemsRangeMonth     = "month"
	ItemsRangeLastMonth = "last-month"
	ItemsRange3Months   = "3m"
	ItemsRange12Months  = "12m"
	ItemsRangeYear      = "year"
	ItemsRangeCustom    = "custom"
)

// ItemsRangePresets are the ranges offered, with their labels.
var ItemsRangePresets = [][2]string{
	{ItemsRangeAll, "All time"},
	{ItemsRangeMonth, "This month"},
	{ItemsRangeLastMonth, "Last month"},
	{ItemsRange3Months, "Last 3 months"},
	{ItemsRange12Months, "Last 12 months"},
	{ItemsRangeYear, "This year"},
	{ItemsRangeCustom, "Custom"},
}

// ParseItemsRange reads ?range= (and ?from= / ?to=, inclusive days, for a
// custom one), today being the user's.
func ParseItemsRange(v url.Values, today time.Time, ms MonthStart) ItemsRange {
	cur := ms.Of(today)
	r := ItemsRange{Key: strings.TrimSpace(v.Get("range"))}
	switch r.Key {
	case ItemsRangeMonth:
		r.From, r.To = cur.From, cur.To
	case ItemsRangeLastMonth:
		p := cur.Prev()
		r.From, r.To = p.From, p.To
	case ItemsRange3Months:
		r.From, r.To = ms.For(cur.Month.AddDate(0, -2, 0)).From, cur.To
	case ItemsRange12Months:
		r.From, r.To = ms.For(cur.Month.AddDate(0, -11, 0)).From, cur.To
	case ItemsRangeYear:
		r.From = time.Date(today.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		r.To = r.From.AddDate(1, 0, 0)
	case ItemsRangeCustom:
		r.From = parseFilterDate(v.Get("from"))
		if to := parseFilterDate(v.Get("to")); !to.IsZero() {
			r.To = to.AddDate(0, 0, 1)
		}
		if !r.From.IsZero() && !r.To.IsZero() && !r.From.Before(r.To) {
			r.From, r.To = r.To.AddDate(0, 0, -1), r.From.AddDate(0, 0, 1)
		}
	default:
		r.Key = ItemsRangeAll
	}
	return r
}

// IsAll tells it counts every day.
func (r ItemsRange) IsAll() bool { return r.From.IsZero() && r.To.IsZero() }

// FromString and ToString are its inclusive days, as typed in the url.
func (r ItemsRange) FromString() string {
	if r.From.IsZero() {
		return ""
	}
	return r.From.Format(txFilterDate)
}

func (r ItemsRange) ToString() string {
	if r.To.IsZero() {
		return ""
	}
	return r.To.AddDate(0, 0, -1).Format(txFilterDate)
}

// Values is it as the page's query string (empty for every day).
func (r ItemsRange) Values() url.Values {
	v := url.Values{}
	if r.Key == ItemsRangeAll || r.Key == "" {
		return v
	}
	v.Set("range", r.Key)
	if r.Key == ItemsRangeCustom {
		if s := r.FromString(); s != "" {
			v.Set("from", s)
		}
		if s := r.ToString(); s != "" {
			v.Set("to", s)
		}
	}
	return v
}

// Label is how it reads, e.g. "Last 3 months" or "Nov 1 – Nov 30, 2026".
func (r ItemsRange) Label() string {
	if r.Key == ItemsRangeCustom {
		switch {
		case r.IsAll():
			return "Any dates"
		case r.To.IsZero():
			return "Since " + r.From.Format("Jan 2, 2006")
		case r.From.IsZero():
			return "Until " + r.To.AddDate(0, 0, -1).Format("Jan 2, 2006")
		}
		return r.From.Format("Jan 2") + " – " + r.To.AddDate(0, 0, -1).Format("Jan 2, 2006")
	}
	for _, p := range ItemsRangePresets {
		if p[0] == r.Key {
			return p[1]
		}
	}
	return "All time"
}

// Days is the inclusive days it spans, "" when open-ended.
func (r ItemsRange) Days() string {
	if r.From.IsZero() || r.To.IsZero() || r.Key == ItemsRangeCustom {
		return ""
	}
	return r.From.Format("Jan 2") + " – " + r.To.AddDate(0, 0, -1).Format("Jan 2, 2006")
}
