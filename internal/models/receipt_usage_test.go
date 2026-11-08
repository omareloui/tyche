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

func TestNewReceiptUsageReport(t *testing.T) {
	at := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 12, 0, 0, 0, time.UTC) }
	scans := []ReceiptScan{
		{TenantID: "GL_1", ScannedAt: at(9, 3), InputTokens: 1000, OutputTokens: 200, CostMicros: 1500, OK: true},
		{TenantID: "GL_1", ScannedAt: at(10, 30), InputTokens: 1000, OutputTokens: 200, CostMicros: 1500, OK: true},
		{TenantID: "GL_2", ScannedAt: at(11, 2), InputTokens: 900, OutputTokens: 100, CostMicros: 1000, OK: true},
		{TenantID: "GL_2", ScannedAt: at(11, 5), CostMicros: 0, OK: false, Error: "timeout"},
		{TenantID: "GL_1", ScannedAt: at(11, 5), InputTokens: 800, OutputTokens: 50, CostMicros: 800, OK: false, Error: "unreadable"},
	}
	now := at(11, 8)
	rep := NewReceiptUsageReport(scans, now, now.AddDate(0, 0, -9), time.Time{}, map[string]int{"GL_3": 25}, 10)

	if rep.AllTime.Scans != 5 || rep.AllTime.CostMicros != 4800 || rep.AllTime.Failed != 2 {
		t.Errorf("all time = %+v", rep.AllTime)
	}
	if rep.ThisMonth.Scans != 3 || rep.LastMonth.Scans != 1 || rep.ThisMonth.InputTokens != 1700 {
		t.Errorf("this month = %+v, last = %+v", rep.ThisMonth, rep.LastMonth)
	}
	if got := rep.AllTime.Cost(); got != 0.0048 {
		t.Errorf("cost = %v", got)
	}

	// Oct 30 → Nov 8: 10 days.
	if len(rep.Daily) != 10 || rep.Daily[0].From != time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("daily = %d from %v", len(rep.Daily), rep.Daily[0].From)
	}
	if rep.Daily[0].Scans != 1 || rep.Daily[6].Scans != 2 || rep.Daily[6].Failed != 2 || rep.Daily[9].Scans != 0 {
		t.Errorf("daily = %+v", rep.Daily)
	}

	// Sep → Nov, from the first scan.
	if len(rep.Monthly) != 3 || rep.Monthly[0].Scans != 1 || rep.Monthly[1].Scans != 1 || rep.Monthly[2].Scans != 3 {
		t.Errorf("monthly = %+v", rep.Monthly)
	}

	if len(rep.Tenants) != 3 {
		t.Fatalf("tenants = %+v", rep.Tenants)
	}
	// GL_2 has 2 scans this month, GL_1 one, GL_3 none.
	gl2, gl1, gl3 := rep.Tenants[0], rep.Tenants[1], rep.Tenants[2]
	if gl1.TenantID != "GL_1" || gl2.TenantID != "GL_2" || gl3.TenantID != "GL_3" {
		t.Fatalf("order = %s %s %s", gl1.TenantID, gl2.TenantID, gl3.TenantID)
	}
	// GL_2's failed scan used no tokens: not billed.
	if gl1.Billed != 1 || gl2.Billed != 1 {
		t.Errorf("billed = %d %d", gl1.Billed, gl2.Billed)
	}
	if gl1.Limit != 10 || gl1.Custom || gl3.Limit != 25 || !gl3.Custom {
		t.Errorf("limits = %+v %+v", gl1, gl3)
	}
}

func TestNewReceiptUsageReportEmpty(t *testing.T) {
	now := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	rep := NewReceiptUsageReport(nil, now, now.AddDate(0, 0, -89), time.Time{}, nil, 10)
	if len(rep.Daily) != 90 || len(rep.Monthly) != 1 || len(rep.Tenants) != 0 {
		t.Errorf("report = %d days, %d months, %d tenants", len(rep.Daily), len(rep.Monthly), len(rep.Tenants))
	}
}
