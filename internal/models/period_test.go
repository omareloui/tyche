package models

import "testing"

func TestMonthStartFor(t *testing.T) {
	tests := []struct {
		start    MonthStart
		month    string
		from, to string
	}{
		{1, "2026-11-01", "2026-11-01", "2026-12-01"},
		{0, "2026-11-15", "2026-11-01", "2026-12-01"},
		{32, "2026-11-15", "2026-11-01", "2026-12-01"},
		{25, "2026-11-01", "2026-10-25", "2026-11-25"},
		{25, "2027-01-01", "2026-12-25", "2027-01-25"},
		{31, "2026-03-01", "2026-02-28", "2026-03-31"},
		{31, "2028-03-01", "2028-02-29", "2028-03-31"},
		{30, "2026-02-01", "2026-01-30", "2026-02-28"},
	}
	for _, tt := range tests {
		p := tt.start.For(day(tt.month))
		if !p.From.Equal(day(tt.from)) || !p.To.Equal(day(tt.to)) {
			t.Errorf("%d.For(%s) = [%s, %s), want [%s, %s)", tt.start, tt.month,
				p.From.Format("2006-01-02"), p.To.Format("2006-01-02"), tt.from, tt.to)
		}
		if p.Key() != tt.month[:7] {
			t.Errorf("%d.For(%s).Key() = %s", tt.start, tt.month, p.Key())
		}
	}
}

func TestMonthStartOf(t *testing.T) {
	tests := []struct {
		start MonthStart
		day   string
		month string
	}{
		{1, "2026-11-30", "2026-11"},
		{25, "2026-11-24", "2026-11"},
		{25, "2026-11-25", "2026-12"},
		{25, "2026-12-31", "2027-01"},
		{31, "2026-02-27", "2026-02"},
		{31, "2026-02-28", "2026-03"},
		{31, "2026-03-30", "2026-03"},
		{31, "2026-03-31", "2026-04"},
	}
	for _, tt := range tests {
		p := tt.start.Of(day(tt.day))
		if p.Key() != tt.month {
			t.Errorf("%d.Of(%s) = %s, want %s", tt.start, tt.day, p.Key(), tt.month)
		}
		if !p.Contains(day(tt.day)) {
			t.Errorf("%d.Of(%s) = [%s, %s) doesn't contain it", tt.start, tt.day, p.From, p.To)
		}
	}
}

func TestPeriod(t *testing.T) {
	p := MonthStart(25).For(day("2026-11-01"))
	if got := p.Days(); got != 31 {
		t.Errorf("Days() = %d, want 31", got)
	}
	if got := p.Day(day("2026-10-27")); got != 2 {
		t.Errorf("Day() = %d, want 2", got)
	}
	if got := p.Prev().Key(); got != "2026-10" {
		t.Errorf("Prev() = %s", got)
	}
	if got := p.Next().From; !got.Equal(day("2026-11-25")) {
		t.Errorf("Next().From = %s", got)
	}
	if got := p.RangeLabel(); got != "Oct 25 – Nov 24" {
		t.Errorf("RangeLabel() = %q", got)
	}
	if p.IsCalendar() || !MonthStart(1).For(day("2026-11-01")).IsCalendar() {
		t.Error("IsCalendar() wrong")
	}
}
