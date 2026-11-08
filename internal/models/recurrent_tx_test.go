package models

import (
	"slices"
	"testing"
	"time"
)

func TestRecurrentOccurrenceOn(t *testing.T) {
	cases := []struct {
		name string
		rt   RecurrentTx
		want []string
	}{
		{"monthly on the 31st", RecurrentTx{Unit: RecurrentUnitMonth, Every: 1, Anchor: day("2026-01-31")},
			[]string{"2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30", "2026-05-31"}},
		{"every 2 months on the 30th", RecurrentTx{Unit: RecurrentUnitMonth, Every: 2, Anchor: day("2025-12-30")},
			[]string{"2025-12-30", "2026-02-28", "2026-04-30"}},
		{"yearly on Feb 29", RecurrentTx{Unit: RecurrentUnitYear, Every: 1, Anchor: day("2028-02-29")},
			[]string{"2028-02-29", "2029-02-28", "2030-02-28", "2031-02-28", "2032-02-29"}},
		{"every 2 weeks", RecurrentTx{Unit: RecurrentUnitDay, Every: 14, Anchor: day("2026-12-25")},
			[]string{"2026-12-25", "2027-01-08", "2027-01-22"}},
	}
	for _, c := range cases {
		for k, w := range c.want {
			if got := c.rt.OccurrenceOn(k); !got.Equal(day(w)) {
				t.Errorf("%s: occurrence %d = %s, want %s", c.name, k, got.Format(time.DateOnly), w)
			}
		}
	}
}

func TestRecurrentResumeIndex(t *testing.T) {
	rt := RecurrentTx{Unit: RecurrentUnitMonth, Every: 1, Anchor: day("2025-01-31")}
	if k := rt.ResumeIndex(day("2026-11-05")); !rt.OccurrenceOn(k).Equal(day("2026-11-30")) {
		t.Errorf("resume = %d (%s), want Nov 30", k, rt.OccurrenceOn(k))
	}
	// A day it happens on is that occurrence.
	if k := rt.ResumeIndex(day("2026-10-31")); !rt.OccurrenceOn(k).Equal(day("2026-10-31")) {
		t.Errorf("resume on an occurrence = %s", rt.OccurrenceOn(k))
	}
	if k := rt.ResumeIndex(day("2024-06-01")); k != 0 {
		t.Errorf("resume before the anchor = %d, want 0", k)
	}
	daily := RecurrentTx{Unit: RecurrentUnitDay, Every: 3, Anchor: day("2026-11-01")}
	if k := daily.ResumeIndex(day("2026-11-05")); k != 2 {
		t.Errorf("every 3 days resume = %d, want 2", k)
	}
}

func TestRecurrentContinueFrom(t *testing.T) {
	today := day("2026-11-05")
	// Migrated with its last one scheduled in June: continues from today.
	stale := RecurrentTx{Unit: RecurrentUnitMonth, Every: 1, Anchor: day("2026-06-15"), NextIndex: 1, NextUnset: true}
	stale.ContinueFrom(today)
	if !stale.NextOn().Equal(day("2026-11-15")) || stale.NextUnset {
		t.Errorf("stale next = %s", stale.NextOn())
	}
	// Last scheduled ahead: continues after it.
	ahead := RecurrentTx{Unit: RecurrentUnitMonth, Every: 1, Anchor: day("2027-01-20"), NextIndex: 1, NextUnset: true}
	ahead.ContinueFrom(today)
	if !ahead.NextOn().Equal(day("2027-02-20")) {
		t.Errorf("ahead next = %s", ahead.NextOn())
	}
	// Not migrated: left alone, overdue ones still written.
	live := RecurrentTx{Unit: RecurrentUnitMonth, Every: 1, Anchor: day("2026-06-15"), NextIndex: 1}
	live.ContinueFrom(today)
	if live.NextIndex != 1 {
		t.Errorf("live next index = %d", live.NextIndex)
	}
}

func TestRecurrentToSchedule(t *testing.T) {
	today := day("2026-11-05")
	horizon := ScheduleHorizon(1, today)
	if !horizon.Equal(day("2026-12-31")) {
		t.Fatalf("horizon = %s, want Dec 31", horizon)
	}
	dates := func(rt RecurrentTx) []time.Time {
		var res []time.Time
		for _, k := range rt.ToSchedule(today, horizon) {
			res = append(res, rt.OccurrenceOn(k))
		}
		return res
	}
	eq := func(got []time.Time, want ...string) bool {
		w := make([]time.Time, len(want))
		for i := range want {
			w[i] = day(want[i])
		}
		return slices.EqualFunc(got, w, time.Time.Equal)
	}

	monthly := RecurrentTx{Unit: RecurrentUnitMonth, Every: 1, Anchor: day("2026-10-25"), NextIndex: 1}
	if got := dates(monthly); !eq(got, "2026-11-25", "2026-12-25") {
		t.Errorf("monthly = %v", got)
	}
	// Already written through the horizon: nothing.
	monthly.NextIndex = 3
	if got := dates(monthly); got != nil {
		t.Errorf("monthly written ahead = %v", got)
	}

	// A yearly one past the horizon still gets its next one written.
	yearly := RecurrentTx{Unit: RecurrentUnitYear, Every: 1, Anchor: day("2026-03-01"), NextIndex: 1}
	if got := dates(yearly); !eq(got, "2027-03-01") {
		t.Errorf("yearly = %v", got)
	}
	yearly.NextIndex = 2
	if got := dates(yearly); got != nil {
		t.Errorf("yearly with one ahead = %v", got)
	}

	stopping := RecurrentTx{Unit: RecurrentUnitDay, Every: 7, Anchor: day("2026-11-01"), NextIndex: 1, StopsAt: day("2026-11-20")}
	if got := dates(stopping); !eq(got, "2026-11-08", "2026-11-15") {
		t.Errorf("weekly until it stops = %v", got)
	}

	paused := monthly
	paused.NextIndex, paused.PausedAt = 1, today
	if got := dates(paused); got != nil {
		t.Errorf("paused = %v", got)
	}
}

func TestMatchesRecurrentOccurrence(t *testing.T) {
	acc := Account{ID: "a"}
	netflix := &Counterparty{ID: "nf", IsMerchant: true}
	landlord := &Counterparty{ID: "ll"}
	occ := &Transaction{Account: acc, Kind: ExpenseTxKind, Amount: 300, Counterparty: netflix, ScheduledTo: day("2026-11-10")}
	at := func(d string, amount float64, cp *Counterparty, kind TxKind) *Transaction {
		return &Transaction{Account: acc, Kind: kind, Amount: amount, Counterparty: cp, ScheduledTo: day(d)}
	}
	cases := []struct {
		name string
		sms  *Transaction
		want bool
	}{
		{"same merchant, new price within the tolerance", at("2026-11-11", 320, netflix, ExpenseTxKind), true},
		{"same merchant, far amount", at("2026-11-10", 5, netflix, ExpenseTxKind), false},
		{"close amount, other merchant", at("2026-11-10", 300, &Counterparty{ID: "x", IsMerchant: true}, ExpenseTxKind), false},
		{"close amount, no counterparty", at("2026-11-10", 300, nil, ExpenseTxKind), false},
		{"2 days early", at("2026-11-08", 300, netflix, ExpenseTxKind), true},
		{"3 days early", at("2026-11-07", 300, netflix, ExpenseTxKind), false},
		{"too late", at("2026-11-13", 300, netflix, ExpenseTxKind), false},
		{"credit for an expense", at("2026-11-10", 300, netflix, IncomeTxKind), false},
		{"other account", &Transaction{Account: Account{ID: "b"}, Kind: ExpenseTxKind, Amount: 300, Counterparty: netflix, ScheduledTo: day("2026-11-10")}, false},
	}
	for _, c := range cases {
		if got := MatchesRecurrentOccurrence(c.sms, occ); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}

	// Paid to a person (not a merchant), the SMS's counterparty doesn't tell.
	rent := &Transaction{Account: acc, Kind: ExpenseTxKind, Amount: 5000, Counterparty: landlord, ScheduledTo: day("2026-11-01")}
	if !MatchesRecurrentOccurrence(at("2026-11-01", 5000, &Counterparty{ID: "bank-name"}, ExpenseTxKind), rent) {
		t.Error("rent to a person not matching its SMS")
	}

	transfer := Transaction{Account: acc, Kind: TransferTxKind, Amount: 1000, ToAmount: 1000, ScheduledTo: day("2026-11-01")}
	if !MatchesRecurrentOccurrence(at("2026-11-02", 1000, nil, ExpenseTxKind), &transfer) {
		t.Error("debit not matching a transfer out")
	}

	cands := []Transaction{
		{ID: "far", Account: acc, Kind: ExpenseTxKind, Amount: 300, ScheduledTo: day("2026-11-08")},
		{ID: "near", Account: acc, Kind: ExpenseTxKind, Amount: 300, ScheduledTo: day("2026-11-09")},
	}
	if got := PickRecurrentOccurrence(at("2026-11-10", 300, nil, ExpenseTxKind), cands); got == nil || got.ID != "near" {
		t.Errorf("picked %v, want near", got)
	}

	// Its counterparty wins over a closer one in date.
	cpCands := []Transaction{
		{ID: "other", Account: acc, Kind: ExpenseTxKind, Amount: 300, ScheduledTo: day("2026-11-10")},
		{ID: "mine", Account: acc, Kind: ExpenseTxKind, Amount: 300, Counterparty: landlord, ScheduledTo: day("2026-11-08")},
	}
	if got := PickRecurrentOccurrence(at("2026-11-10", 300, landlord, ExpenseTxKind), cpCands); got == nil || got.ID != "mine" {
		t.Errorf("picked %v, want mine", got)
	}

	sms := at("2026-11-11", 350, &Counterparty{ID: "sms"}, ExpenseTxKind)
	sms.FulfilledAt = day("2026-11-11")
	merged := *occ
	merged.MergeSMS(sms)
	if merged.Amount != 350 || !merged.ScheduledTo.Equal(sms.ScheduledTo) || merged.FulfilledAt.IsZero() || !merged.NeedsReview || merged.Counterparty != netflix {
		t.Errorf("merged = %+v", merged)
	}
}
