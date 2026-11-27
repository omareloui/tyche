package models

import (
	"net/url"
	"testing"
)

func TestTxFilterTimeOfDay(t *testing.T) {
	f := ParseTxFilter(url.Values{TxFilterTimeFrom: {"22:00"}, TxFilterTimeTo: {"bad"}})
	if f.TimeFrom == nil || *f.TimeFrom != 22*60 || f.TimeTo != nil {
		t.Fatalf("parsed %v %v", f.TimeFrom, f.TimeTo)
	}
	if from, to, ok := f.TimeRange(); !ok || from != 22*60 || to != 24*60-1 {
		t.Errorf("range %d %d %v", from, to, ok)
	}
	if got := f.Values().Encode(); got != "time_from=22%3A00" {
		t.Errorf("values %q", got)
	}
	if f.ActiveCount() != 1 {
		t.Errorf("active %d", f.ActiveCount())
	}
	var zero TxFilter
	if zero.HasTime() || !zero.IsZero() {
		t.Error("zero filter filters the time")
	}
	if got := FilterTimeString(new(int)); got != "00:00" {
		t.Errorf("midnight %q", got)
	}
}
