package models

import (
	"slices"
	"time"
)

// ReceiptScan is a call reading a receipt, as kept for the usage.
type ReceiptScan struct {
	TenantID     string
	ScannedAt    time.Time
	Model        string
	InputTokens  int
	OutputTokens int
	// CostMicros is in millionths of a dollar, at the prices of the scan.
	CostMicros int64
	OK         bool
	Error      string
	// Cached tells it was answered from what was read off the same image
	// before (the tenant's receipts), without a call.
	Cached bool
}

// Billed tells the scan used tokens, counting toward the monthly limit.
func (s *ReceiptScan) Billed() bool {
	return !s.Cached && (s.OK || s.InputTokens > 0 || s.OutputTokens > 0)
}

// CalendarMonth is the calendar month (UTC) t is in, [from, to).
func CalendarMonth(t time.Time) (from, to time.Time) {
	y, m, _ := t.UTC().Date()
	from = time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	return from, from.AddDate(0, 1, 0)
}

// ReceiptUsage sums scans.
type ReceiptUsage struct {
	Scans  int
	Failed int
	// Cached are the scans answered from the cache, apart: they aren't
	// scans (no call, not counted toward the limit).
	Cached       int
	InputTokens  int
	OutputTokens int
	CostMicros   int64
	Last         time.Time
}

func (u *ReceiptUsage) add(s *ReceiptScan) {
	if s.Cached {
		u.Cached++
		return
	}
	u.Scans++
	if !s.OK {
		u.Failed++
	}
	u.InputTokens += s.InputTokens
	u.OutputTokens += s.OutputTokens
	u.CostMicros += s.CostMicros
	if s.ScannedAt.After(u.Last) {
		u.Last = s.ScannedAt
	}
}

// Cost is the usage's cost in dollars.
func (u *ReceiptUsage) Cost() float64 { return float64(u.CostMicros) / 1e6 }

// AvgCost is the cost of a scan on average, in dollars.
func (u *ReceiptUsage) AvgCost() float64 {
	if u.Scans == 0 {
		return 0
	}
	return u.Cost() / float64(u.Scans)
}

// ReceiptUsageBucket is a day's or month's usage, From its first day.
type ReceiptUsageBucket struct {
	From time.Time
	ReceiptUsage
}

// TenantReceiptUsage is a tenant's usage this month and in all.
type TenantReceiptUsage struct {
	TenantID  string
	ThisMonth ReceiptUsage
	AllTime   ReceiptUsage
	// Limit is its scans a calendar month, Custom when set by an admin.
	Limit  int
	Custom bool
	// Billed is its scans counting toward the limit this month.
	Billed int
}

// ReceiptUsageReport is the usage of every scan, for the admin page.
type ReceiptUsageReport struct {
	ThisMonth, LastMonth, AllTime ReceiptUsage
	// Daily is every day of the range (zero ones too), Monthly every month
	// from the first scan (or the range's start).
	Daily   []ReceiptUsageBucket
	Monthly []ReceiptUsageBucket
	// Tenants are every tenant with scans or a limit set, most scans this
	// month first.
	Tenants []TenantReceiptUsage
}

// NewReceiptUsageReport sums the scans (oldest first) as of now: the days
// since dailyFrom, the months since monthlyFrom (zero for the first scan's),
// each tenant's with its limit (def unless set in limits).
func NewReceiptUsageReport(scans []ReceiptScan, now, dailyFrom, monthlyFrom time.Time, limits map[string]int, def int) ReceiptUsageReport {
	var rep ReceiptUsageReport
	thisFrom, thisTo := CalendarMonth(now)
	lastFrom := thisFrom.AddDate(0, -1, 0)

	dayOf := func(t time.Time) time.Time {
		y, m, d := t.UTC().Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	dailyFrom = dayOf(dailyFrom)
	today := dayOf(now)
	for d := dailyFrom; !d.After(today); d = d.AddDate(0, 0, 1) {
		rep.Daily = append(rep.Daily, ReceiptUsageBucket{From: d})
	}

	if monthlyFrom.IsZero() && len(scans) > 0 {
		monthlyFrom = scans[0].ScannedAt
	}
	if monthlyFrom.IsZero() {
		monthlyFrom = now
	}
	monthlyFrom, _ = CalendarMonth(monthlyFrom)
	for m := monthlyFrom; !m.After(thisFrom); m = m.AddDate(0, 1, 0) {
		rep.Monthly = append(rep.Monthly, ReceiptUsageBucket{From: m})
	}

	tenants := map[string]*TenantReceiptUsage{}
	tenantOf := func(id string) *TenantReceiptUsage {
		t, ok := tenants[id]
		if !ok {
			t = &TenantReceiptUsage{TenantID: id, Limit: def}
			if l, ok := limits[id]; ok {
				t.Limit, t.Custom = l, true
			}
			tenants[id] = t
		}
		return t
	}
	for id := range limits {
		tenantOf(id)
	}

	for i := range scans {
		s := &scans[i]
		at := s.ScannedAt.UTC()
		rep.AllTime.add(s)
		t := tenantOf(s.TenantID)
		t.AllTime.add(s)
		if !at.Before(thisFrom) && at.Before(thisTo) {
			rep.ThisMonth.add(s)
			t.ThisMonth.add(s)
			if s.Billed() {
				t.Billed++
			}
		} else if !at.Before(lastFrom) && at.Before(thisFrom) {
			rep.LastMonth.add(s)
		}
		if n := int(dayOf(at).Sub(dailyFrom).Hours() / 24); n >= 0 && n < len(rep.Daily) {
			rep.Daily[n].add(s)
		}
		if n := (at.Year()-monthlyFrom.Year())*12 + int(at.Month()-monthlyFrom.Month()); n >= 0 && n < len(rep.Monthly) {
			rep.Monthly[n].add(s)
		}
	}

	for _, t := range tenants {
		rep.Tenants = append(rep.Tenants, *t)
	}
	slices.SortFunc(rep.Tenants, func(a, b TenantReceiptUsage) int {
		if a.ThisMonth.Scans != b.ThisMonth.Scans {
			return b.ThisMonth.Scans - a.ThisMonth.Scans
		}
		if a.AllTime.Scans != b.AllTime.Scans {
			return b.AllTime.Scans - a.AllTime.Scans
		}
		if a.TenantID < b.TenantID {
			return -1
		}
		return 1
	})
	return rep
}
