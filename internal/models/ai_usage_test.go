package models

import (
	"testing"
	"time"
)

func TestCalendarMonth(t *testing.T) {
	cairo, _ := time.LoadLocation("Africa/Cairo")
	// Dec 1st 00:30 in Cairo is still November in UTC.
	from, to := CalendarMonth(time.Date(2026, 12, 1, 0, 30, 0, 0, cairo))
	if from != time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC) || to != time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC) {
		t.Errorf("month = %v – %v", from, to)
	}
}

var testDefs = map[ScanKind]int{ScanReceipt: 10, ScanSMSGuess: 5, ScanColEstimate: 2}

func TestNewAIUsageReport(t *testing.T) {
	at := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 12, 0, 0, 0, time.UTC) }
	scans := []ReceiptScan{
		{TenantID: "GL_1", ScannedAt: at(9, 3), InputTokens: 1000, OutputTokens: 200, CostMicros: 1500, OK: true},
		{TenantID: "GL_1", ScannedAt: at(10, 30), InputTokens: 1000, OutputTokens: 200, CostMicros: 1500, OK: true},
		{TenantID: "GL_2", ScannedAt: at(11, 2), InputTokens: 900, OutputTokens: 100, CostMicros: 1000, OK: true},
		{TenantID: "GL_2", ScannedAt: at(11, 5), CostMicros: 0, OK: false, Error: "timeout"},
		{TenantID: "GL_1", ScannedAt: at(11, 5), InputTokens: 800, OutputTokens: 50, CostMicros: 800, OK: false, Error: "unreadable"},
		// A cache hit isn't a call.
		{TenantID: "GL_1", ScannedAt: at(11, 6), OK: true, Cached: true},
	}
	now := at(11, 8)
	rep := NewAIUsageReport(scans, now, now.AddDate(0, 0, -9), time.Time{}, map[string]map[ScanKind]int{"GL_3": {ScanReceipt: 25}}, testDefs)

	if rep.AllTime.Calls != 5 || rep.AllTime.CostMicros != 4800 || rep.AllTime.Failed != 2 || rep.AllTime.Cached != 1 || rep.ThisMonth.Cached != 1 {
		t.Errorf("all time = %+v", rep.AllTime)
	}
	if rep.ThisMonth.Calls != 3 || rep.LastMonth.Calls != 1 || rep.ThisMonth.InputTokens != 1700 {
		t.Errorf("this month = %+v, last = %+v", rep.ThisMonth, rep.LastMonth)
	}
	if got := rep.AllTime.Cost(); got != 0.0048 {
		t.Errorf("cost = %v", got)
	}
	if rep.AllTime.Kind(ScanReceipt).Calls != 5 || rep.AllTime.Kind(ScanSMSGuess).Calls != 0 {
		t.Errorf("by kind = %+v", rep.AllTime)
	}

	// Oct 30 → Nov 8: 10 days.
	if len(rep.Daily) != 10 || rep.Daily[0].From != time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("daily = %d from %v", len(rep.Daily), rep.Daily[0].From)
	}
	if rep.Daily[7].Calls != 0 || rep.Daily[7].Cached != 1 {
		t.Errorf("the cached day = %+v", rep.Daily[7])
	}
	if rep.Daily[0].Calls != 1 || rep.Daily[6].Calls != 2 || rep.Daily[6].Failed != 2 || rep.Daily[9].Calls != 0 {
		t.Errorf("daily = %+v", rep.Daily)
	}

	// Sep → Nov, from the first call.
	if len(rep.Monthly) != 3 || rep.Monthly[0].Calls != 1 || rep.Monthly[1].Calls != 1 || rep.Monthly[2].Calls != 3 {
		t.Errorf("monthly = %+v", rep.Monthly)
	}

	if len(rep.Tenants) != 3 {
		t.Fatalf("tenants = %+v", rep.Tenants)
	}
	// GL_2 cost 1000 this month, GL_1 800, GL_3 nothing.
	gl2, gl1, gl3 := rep.Tenants[0], rep.Tenants[1], rep.Tenants[2]
	if gl1.TenantID != "GL_1" || gl2.TenantID != "GL_2" || gl3.TenantID != "GL_3" {
		t.Fatalf("order = %s %s %s", gl1.TenantID, gl2.TenantID, gl3.TenantID)
	}
	// GL_2's failed call used no tokens: not billed.
	if gl1.Billed[ScanReceipt] != 1 || gl2.Billed[ScanReceipt] != 1 {
		t.Errorf("billed = %v %v", gl1.Billed, gl2.Billed)
	}
	if gl1.Limits[ScanReceipt] != 10 || gl1.Custom[ScanReceipt] || gl3.Limits[ScanReceipt] != 25 || !gl3.Custom[ScanReceipt] || gl3.Limits[ScanSMSGuess] != 5 || gl3.Custom[ScanSMSGuess] {
		t.Errorf("limits = %+v %+v", gl1, gl3)
	}
}

func TestNewAIUsageReportEmpty(t *testing.T) {
	now := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	rep := NewAIUsageReport(nil, now, now.AddDate(0, 0, -89), time.Time{}, nil, testDefs)
	if len(rep.Daily) != 90 || len(rep.Monthly) != 1 || len(rep.Tenants) != 0 || rep.MonthEndMicros != 0 || rep.WorstCaseMicros != 0 {
		t.Errorf("report = %d days, %d months, %d tenants", len(rep.Daily), len(rep.Monthly), len(rep.Tenants))
	}
}

func TestNewAIUsageReportKinds(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	scans := []ReceiptScan{
		{TenantID: "GL_1", ScannedAt: now, InputTokens: 1000, CostMicros: 1500, OK: true},
		{TenantID: "GL_1", ScannedAt: now, InputTokens: 900, CostMicros: 1000, OK: true, Kind: ScanSMSGuess},
		{TenantID: "GL_1", ScannedAt: now, InputTokens: 500, SearchQueries: 2, CostMicros: 28500, OK: true, Kind: ScanColEstimate},
	}
	rep := NewAIUsageReport(scans, now, now, time.Time{}, nil, testDefs)
	if rep.ThisMonth.Calls != 3 || rep.ThisMonth.CostMicros != 31000 || rep.ThisMonth.SearchQueries != 2 {
		t.Errorf("this month = %+v", rep.ThisMonth)
	}
	if k := rep.ThisMonth.Kind(ScanSMSGuess); k.Calls != 1 || k.CostMicros != 1000 {
		t.Errorf("sms = %+v", k)
	}
	if k := rep.Daily[0].Kind(ScanColEstimate); k.Calls != 1 || k.SearchQueries != 2 {
		t.Errorf("col = %+v", k)
	}
	gl1 := rep.Tenants[0]
	// Each kind counts toward its own limit.
	if gl1.Billed[ScanReceipt] != 1 || gl1.Billed[ScanSMSGuess] != 1 || gl1.Billed[ScanColEstimate] != 1 {
		t.Errorf("billed = %v", gl1.Billed)
	}
}

func TestAIUsageProjection(t *testing.T) {
	// Nov 16 00:00: half of November (30 days) gone.
	now := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	scans := []ReceiptScan{
		{TenantID: "GL_1", ScannedAt: now.Add(-time.Hour), InputTokens: 1, CostMicros: 1000, OK: true},
		{TenantID: "GL_1", ScannedAt: now.Add(-time.Hour), InputTokens: 1, CostMicros: 3000, OK: true, Kind: ScanSMSGuess},
	}
	rep := NewAIUsageReport(scans, now, now, time.Time{}, map[string]map[ScanKind]int{"GL_2": {ScanReceipt: 0}}, testDefs)
	if rep.MonthEndMicros != 8000 || rep.Tenants[0].MonthEndMicros != 8000 {
		t.Errorf("month end = %d / %d", rep.MonthEndMicros, rep.Tenants[0].MonthEndMicros)
	}
	// GL_1: 4000 + 9 receipts × 1000 + 4 guesses × 3000 (no city estimate
	// was ever made: 0). GL_2: 0 receipts + 5 guesses × 3000.
	if gl1 := rep.Tenants[0]; gl1.WorstCaseMicros != 25000 {
		t.Errorf("GL_1 worst case = %d", gl1.WorstCaseMicros)
	}
	if rep.WorstCaseMicros != 25000+15000 {
		t.Errorf("worst case = %d", rep.WorstCaseMicros)
	}

	// The first hours count as a day.
	early := time.Date(2026, 11, 1, 1, 0, 0, 0, time.UTC)
	rep = NewAIUsageReport([]ReceiptScan{{TenantID: "GL_1", ScannedAt: early, InputTokens: 1, CostMicros: 1000, OK: true}}, early, early, time.Time{}, nil, testDefs)
	if rep.MonthEndMicros != 30000 {
		t.Errorf("early month end = %d", rep.MonthEndMicros)
	}
}

func TestParseScanKind(t *testing.T) {
	if k, ok := ParseScanKind("sms"); !ok || k != ScanSMSGuess {
		t.Errorf("sms = %q %v", k, ok)
	}
	if _, ok := ParseScanKind("nope"); ok {
		t.Error("nope parsed")
	}
}
