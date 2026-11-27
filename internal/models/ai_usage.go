package models

import (
	"cmp"
	"math"
	"slices"
	"time"
)

// ReceiptScan is an AI (Gemini) call, of any kind, as kept for the usage.
type ReceiptScan struct {
	TenantID     string
	ScannedAt    time.Time
	Model        string
	InputTokens  int
	OutputTokens int
	// SearchQueries are the Google Search queries a grounded call ran.
	SearchQueries int
	// CostMicros is in millionths of a dollar, at the prices of the call.
	CostMicros int64
	OK         bool
	Error      string
	// Cached tells it was answered from what was read off the same image
	// before (the tenant's receipts), without a call.
	Cached bool
	// Kind is what was read, ScanReceipt when empty.
	Kind ScanKind
}

// ScanKind is what a Gemini call read, each counted toward its own monthly
// limit.
type ScanKind string

const (
	ScanReceipt ScanKind = "receipt"
	// ScanSMSGuess guessed an sms' template.
	ScanSMSGuess ScanKind = "sms"
	// ScanColEstimate estimated a city's prices (cost of living).
	ScanColEstimate ScanKind = "col"
	// ScanTaxRules looked up a country's tax rules (salary calculator).
	ScanTaxRules ScanKind = "tax"
	// ScanVisaRules looked up a visa's requirements (visa readiness).
	ScanVisaRules ScanKind = "visa"
	// ScanMerchant matched the unknown merchants of an sms batch.
	ScanMerchant ScanKind = "merchant"
	// ScanQuickAdd read a quick add's text into transactions.
	ScanQuickAdd ScanKind = "quick"
	// ScanSplit read who had what of a split bill from a sentence.
	ScanSplit ScanKind = "split"
)

// ScanKinds are every kind of AI call, in the order shown.
var ScanKinds = []ScanKind{ScanReceipt, ScanSMSGuess, ScanColEstimate, ScanTaxRules, ScanVisaRules, ScanMerchant, ScanQuickAdd, ScanSplit}

// ParseScanKind is the kind named s, false when there is none.
func ParseScanKind(s string) (ScanKind, bool) {
	k := ScanKind(s)
	return k, slices.Contains(ScanKinds, k)
}

// Label names the kind's feature.
func (k ScanKind) Label() string {
	switch k {
	case ScanSMSGuess:
		return "SMS guesses"
	case ScanColEstimate:
		return "City estimates"
	case ScanTaxRules:
		return "Tax rules"
	case ScanVisaRules:
		return "Visa rules"
	case ScanMerchant:
		return "Merchant matches"
	case ScanQuickAdd:
		return "Quick adds"
	case ScanSplit:
		return "Bill splits"
	default:
		return "Receipt scans"
	}
}

func (k ScanKind) index() int {
	return max(slices.Index(ScanKinds, cmp.Or(k, ScanReceipt)), 0)
}

// Billed tells the call used tokens (or searches), counting toward the
// monthly limit.
func (s *ReceiptScan) Billed() bool {
	return !s.Cached && (s.OK || s.InputTokens > 0 || s.OutputTokens > 0 || s.SearchQueries > 0)
}

// CalendarMonth is the calendar month (UTC) t is in, [from, to).
func CalendarMonth(t time.Time) (from, to time.Time) {
	y, m, _ := t.UTC().Date()
	from = time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	return from, from.AddDate(0, 1, 0)
}

// AIUsageTotals sums calls.
type AIUsageTotals struct {
	// Calls are the calls made (failed ones too), without the Cached ones:
	// answered from the cache, they aren't calls (nothing billed, not
	// counted toward the limit).
	Calls         int
	Failed        int
	Cached        int
	InputTokens   int
	OutputTokens  int
	SearchQueries int
	CostMicros    int64
}

func (u *AIUsageTotals) add(s *ReceiptScan) {
	if s.Cached {
		u.Cached++
		return
	}
	u.Calls++
	if !s.OK {
		u.Failed++
	}
	u.InputTokens += s.InputTokens
	u.OutputTokens += s.OutputTokens
	u.SearchQueries += s.SearchQueries
	u.CostMicros += s.CostMicros
}

// Cost is the usage's cost in dollars.
func (u AIUsageTotals) Cost() float64 { return float64(u.CostMicros) / 1e6 }

// AvgCost is the cost of a call on average, in dollars.
func (u AIUsageTotals) AvgCost() float64 {
	if u.Calls == 0 {
		return 0
	}
	return u.Cost() / float64(u.Calls)
}

// AIUsage sums calls, in all and of each kind.
type AIUsage struct {
	AIUsageTotals
	Last time.Time
	// kinds is per ScanKinds (its length: grow it with them).
	kinds [8]AIUsageTotals
}

func (u *AIUsage) add(s *ReceiptScan) {
	u.AIUsageTotals.add(s)
	u.kinds[s.Kind.index()].add(s)
	if !s.Cached && s.ScannedAt.After(u.Last) {
		u.Last = s.ScannedAt
	}
}

// Kind is the usage of the kind's calls.
func (u AIUsage) Kind(k ScanKind) AIUsageTotals { return u.kinds[k.index()] }

// AIUsageBucket is a day's or month's usage, From its first day.
type AIUsageBucket struct {
	From time.Time
	AIUsage
}

// TenantAIUsage is a tenant's usage this month and in all.
type TenantAIUsage struct {
	TenantID  string
	ThisMonth AIUsage
	AllTime   AIUsage
	// Limits are its calls a calendar month of each kind, Custom the ones
	// set by an admin.
	Limits map[ScanKind]int
	Custom map[ScanKind]bool
	// Billed are its calls of each kind counting toward the limits this
	// month.
	Billed map[ScanKind]int
	// MonthEndMicros is this month's cost at its pace so far by the month's
	// end, WorstCaseMicros if it used what's left of every limit.
	MonthEndMicros  int64
	WorstCaseMicros int64
}

// AIUsageReport is the usage of every AI call, for the admin pages.
type AIUsageReport struct {
	ThisMonth, LastMonth, AllTime AIUsage
	// Daily is every day of the range (zero ones too), Monthly every month
	// from the first call (or the range's start).
	Daily   []AIUsageBucket
	Monthly []AIUsageBucket
	// Tenants are every tenant with calls or a limit set, the most costly
	// this month first.
	Tenants []TenantAIUsage
	// MonthEndMicros is this month's cost at its pace so far by the month's
	// end; WorstCaseMicros if every listed tenant used what's left of its
	// limits (each kind at its average cost).
	MonthEndMicros  int64
	WorstCaseMicros int64
}

// MonthEndCost is MonthEndMicros in dollars.
func (r AIUsageReport) MonthEndCost() float64 { return float64(r.MonthEndMicros) / 1e6 }

// WorstCaseCost is WorstCaseMicros in dollars.
func (r AIUsageReport) WorstCaseCost() float64 { return float64(r.WorstCaseMicros) / 1e6 }

// MonthEndCost is MonthEndMicros in dollars.
func (t TenantAIUsage) MonthEndCost() float64 { return float64(t.MonthEndMicros) / 1e6 }

// monthElapsed is the share of now's calendar month gone, a day at least
// so the first hours don't make wild projections.
func monthElapsed(now time.Time) float64 {
	from, to := CalendarMonth(now)
	gone := max(now.Sub(from), 24*time.Hour)
	return min(gone.Hours()/to.Sub(from).Hours(), 1)
}

// projectMonth is the month's cost so far at its pace by the month's end.
func projectMonth(costMicros int64, elapsed float64) int64 {
	return int64(math.Round(float64(costMicros) / elapsed))
}

// NewAIUsageReport sums the calls (oldest first) as of now: the days since
// dailyFrom, the months since monthlyFrom (zero for the first call's), each
// tenant's with its limits (defs unless set in limits).
func NewAIUsageReport(scans []ReceiptScan, now, dailyFrom, monthlyFrom time.Time, limits map[string]map[ScanKind]int, defs map[ScanKind]int) AIUsageReport {
	var rep AIUsageReport
	thisFrom, thisTo := CalendarMonth(now)
	lastFrom := thisFrom.AddDate(0, -1, 0)

	dayOf := func(t time.Time) time.Time {
		y, m, d := t.UTC().Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	dailyFrom = dayOf(dailyFrom)
	today := dayOf(now)
	for d := dailyFrom; !d.After(today); d = d.AddDate(0, 0, 1) {
		rep.Daily = append(rep.Daily, AIUsageBucket{From: d})
	}

	if monthlyFrom.IsZero() && len(scans) > 0 {
		monthlyFrom = scans[0].ScannedAt
	}
	if monthlyFrom.IsZero() {
		monthlyFrom = now
	}
	monthlyFrom, _ = CalendarMonth(monthlyFrom)
	for m := monthlyFrom; !m.After(thisFrom); m = m.AddDate(0, 1, 0) {
		rep.Monthly = append(rep.Monthly, AIUsageBucket{From: m})
	}

	tenants := map[string]*TenantAIUsage{}
	tenantOf := func(id string) *TenantAIUsage {
		t, ok := tenants[id]
		if !ok {
			t = &TenantAIUsage{TenantID: id, Limits: map[ScanKind]int{}, Custom: map[ScanKind]bool{}, Billed: map[ScanKind]int{}}
			for _, k := range ScanKinds {
				t.Limits[k] = defs[k]
				if l, ok := limits[id][k]; ok {
					t.Limits[k], t.Custom[k] = l, true
				}
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
				t.Billed[cmp.Or(s.Kind, ScanReceipt)]++
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

	elapsed := monthElapsed(now)
	rep.MonthEndMicros = projectMonth(rep.ThisMonth.CostMicros, elapsed)
	rep.WorstCaseMicros = rep.ThisMonth.CostMicros
	for _, t := range tenants {
		t.MonthEndMicros = projectMonth(t.ThisMonth.CostMicros, elapsed)
		t.WorstCaseMicros = t.ThisMonth.CostMicros
		for _, k := range ScanKinds {
			left := max(t.Limits[k]-t.Billed[k], 0)
			avg := rep.AllTime.Kind(k).AvgCost() * 1e6
			t.WorstCaseMicros += int64(math.Round(float64(left) * avg))
		}
		rep.WorstCaseMicros += t.WorstCaseMicros - t.ThisMonth.CostMicros
		rep.Tenants = append(rep.Tenants, *t)
	}
	slices.SortFunc(rep.Tenants, func(a, b TenantAIUsage) int {
		return cmp.Or(
			cmp.Compare(b.ThisMonth.CostMicros, a.ThisMonth.CostMicros),
			cmp.Compare(b.ThisMonth.Calls, a.ThisMonth.Calls),
			cmp.Compare(b.AllTime.CostMicros, a.AllTime.CostMicros),
			cmp.Compare(a.TenantID, b.TenantID),
		)
	})
	return rep
}
