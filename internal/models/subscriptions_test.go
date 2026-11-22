package models

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// subTxs makes charges at cp (EGP) on the given days, each of the matching
// amount (the last amount repeating).
func subTxs(cp string, days []string, amounts ...float64) []SubscriptionTx {
	out := make([]SubscriptionTx, len(days))
	for i, d := range days {
		a := amounts[min(i, len(amounts)-1)]
		out[i] = SubscriptionTx{
			ID: fmt.Sprintf("%s-%02d", cp, i), AccountID: "acc", CurrencyID: "egp",
			CounterpartyID: cp, Day: day(d), Amount: a,
		}
	}
	return out
}

func TestDetectSubscriptionsMonthly(t *testing.T) {
	// Charged around the 3rd, slipping to the next working day.
	txs := subTxs("netflix", []string{"2026-06-03", "2026-07-05", "2026-08-03", "2026-09-04", "2026-10-03", "2026-11-03"}, 200)
	got := DetectSubscriptions(txs, day("2026-11-22"))
	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1", len(got))
	}
	c := got[0]
	if c.Unit != RecurrentUnitMonth || c.Every != 1 {
		t.Errorf("interval = every %d %s, want monthly", c.Every, c.Unit)
	}
	if c.Amount != 200 || c.Count != 6 || c.Change != 0 {
		t.Errorf("amount %v count %d change %v", c.Amount, c.Count, c.Change)
	}
	if !c.Next.Equal(day("2026-12-03")) {
		t.Errorf("next = %s, want 2026-12-03", c.Next.Format(time.DateOnly))
	}
	if got := c.NextFrom(day("2026-11-22")); !got.Equal(day("2026-12-03")) {
		t.Errorf("next from today = %s", got.Format(time.DateOnly))
	}
}

func TestDetectSubscriptionsPriceRise(t *testing.T) {
	txs := subTxs("spotify", []string{"2026-07-10", "2026-08-10", "2026-09-10", "2026-10-10", "2026-11-10"}, 100, 100, 100, 100, 130)
	got := DetectSubscriptions(txs, day("2026-11-22"))
	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1", len(got))
	}
	if c := got[0]; math.Abs(c.Change-0.3) > 1e-9 || c.PrevAmount != 100 || c.LastAmount != 130 {
		t.Errorf("change = %v (prev %v, last %v), want +30%%", c.Change, c.PrevAmount, c.LastAmount)
	}
}

func TestDetectSubscriptionsWeeklyAndYearly(t *testing.T) {
	weekly := subTxs("gym", []string{"2026-10-25", "2026-11-01", "2026-11-08", "2026-11-15"}, 50)
	yearly := subTxs("domain", []string{"2023-11-01", "2024-11-02"}, 600)
	got := DetectSubscriptions(append(weekly, yearly...), day("2026-11-22"))
	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1 (the domain's renewal is overdue a year)", len(got))
	}
	if c := got[0]; c.CounterpartyID != "gym" || c.Every != 7 || c.Unit != RecurrentUnitDay {
		t.Errorf("got %+v, want weekly gym", c)
	}

	yearly = subTxs("domain", []string{"2024-12-20", "2025-12-18"}, 600)
	got = DetectSubscriptions(yearly, day("2026-11-22"))
	if len(got) != 1 || got[0].Unit != RecurrentUnitYear {
		t.Fatalf("got %+v, want a yearly one", got)
	}
}

func TestDetectSubscriptionsStopped(t *testing.T) {
	txs := subTxs("old", []string{"2026-03-01", "2026-04-01", "2026-05-01", "2026-06-01"}, 99)
	if got := DetectSubscriptions(txs, day("2026-11-22")); len(got) != 0 {
		t.Errorf("got %+v, want none: it stopped in June", got)
	}
}

func TestDetectSubscriptionsNoise(t *testing.T) {
	// Groceries: often, any amount.
	days := []string{"2026-10-01", "2026-10-04", "2026-10-09", "2026-10-15", "2026-10-16", "2026-10-28", "2026-11-02", "2026-11-11"}
	txs := subTxs("market", days, 340, 1200, 85, 560, 230, 990, 410, 150)
	if got := DetectSubscriptions(txs, day("2026-11-22")); len(got) != 0 {
		t.Errorf("got %+v, want none", got)
	}
	// Too few charges.
	txs = subTxs("new", []string{"2026-10-05", "2026-11-05"}, 100)
	if got := DetectSubscriptions(txs, day("2026-11-22")); len(got) != 0 {
		t.Errorf("got %+v, want none from two monthly charges", got)
	}
}

func TestDetectSubscriptionsTwoPlansAndOneOff(t *testing.T) {
	days := []string{"2026-08-05", "2026-09-05", "2026-10-05", "2026-11-05"}
	txs := subTxs("apple", days, 50)
	txs = append(txs, subTxs("apple", []string{"2026-08-20", "2026-09-20", "2026-10-20", "2026-11-20"}, 300)...)
	for i := range txs[4:] {
		txs[4+i].ID = fmt.Sprintf("apple-b%d", i)
	}
	// A one off purchase at the same merchant.
	txs = append(txs, SubscriptionTx{ID: "apple-x", CurrencyID: "egp", CounterpartyID: "apple", Day: day("2026-10-12"), Amount: 15000})

	got := DetectSubscriptions(txs, day("2026-11-22"))
	if len(got) != 2 {
		t.Fatalf("got %d candidates, want the two plans", len(got))
	}
	if got[0].Amount != 50 || got[1].Amount != 300 {
		t.Errorf("amounts %v, %v, want 50 then 300 (soonest next first)", got[0].Amount, got[1].Amount)
	}
}

func TestSubscriptionMonthly(t *testing.T) {
	c := SubscriptionCandidate{Amount: 70, Every: 7, Unit: RecurrentUnitDay}
	if m := c.Monthly(); math.Abs(m-70*daysPerMonth/7) > 1e-9 {
		t.Errorf("monthly = %v", m)
	}
	if !c.SameAs(80) || c.SameAs(90) {
		t.Error("SameAs should take 80 and not 90 for 70")
	}
}
