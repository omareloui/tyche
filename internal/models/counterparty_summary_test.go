package models

import (
	"slices"
	"testing"
)

func counterpartySummaryFixture(hidden []string) *CounterpartySummary {
	egp := Currency{String: "EGP"}
	xyz := Currency{String: "XYZ"} // no rate
	cafe := Counterparty{ID: "cafe", Name: "Cafe", Labels: []string{"Food", "Coffee"}}
	shop := Counterparty{ID: "shop", Name: "Shop", Labels: []string{"food"}}
	gym := Counterparty{ID: "gym", Name: "Gym"}
	back := Counterparty{ID: "back", Name: "Refunded"}
	// Months start on the 25th: November is Oct 25 – Nov 24.
	return NewCounterpartySummary(MonthStart(25).For(day("2026-11-01")), egp, []DayMerchantTotal{
		{day("2026-10-25"), cafe, egp, 100},
		{day("2026-11-24"), cafe, egp, 50},
		{day("2026-11-01"), shop, egp, 300},
		{day("2026-11-02"), gym, egp, 80},
		{day("2026-11-03"), back, egp, 40},
		{day("2026-11-04"), back, egp, -40},
		{day("2026-11-05"), gym, xyz, 5},
		// Out of the month.
		{day("2026-10-24"), cafe, egp, 999},
		{day("2026-11-25"), shop, egp, 999},
	}, hidden)
}

func merchantIDs(items []MerchantSpend) []string {
	var res []string
	for _, it := range items {
		res = append(res, it.Counterparty.ID)
	}
	return res
}

func TestNewCounterpartySummary(t *testing.T) {
	cs := counterpartySummaryFixture(nil)
	// Most first, the one refunded in full dropped.
	if got, want := merchantIDs(cs.Items), []string{"shop", "cafe", "gym"}; !slices.Equal(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
	if cs.Total() != 530 || !cs.Partial {
		t.Errorf("total = %v, partial = %v", cs.Total(), cs.Partial)
	}

	// A merchant counts in each of its labels, merged by case (written as the
	// top merchant does), the unlabeled ones last.
	want := []LabelSpend{{"food", 450, 2}, {"Coffee", 150, 1}, {"", 80, 1}}
	if !slices.Equal(cs.Labels, want) || !cs.HasLabels() {
		t.Errorf("labels = %v, want %v", cs.Labels, want)
	}

}

func TestNewCounterpartySummaryHidden(t *testing.T) {
	cs := counterpartySummaryFixture([]string{"shop", "unknown"})
	if got := merchantIDs(cs.Items); !slices.Equal(got, []string{"cafe", "gym"}) {
		t.Fatalf("items = %v", got)
	}
	if got := merchantIDs(cs.Hidden); !slices.Equal(got, []string{"shop"}) {
		t.Errorf("hidden = %v", got)
	}
	if cs.Total() != 230 {
		t.Errorf("total = %v, want 230", cs.Total())
	}
	if want := []LabelSpend{{"Food", 150, 1}, {"Coffee", 150, 1}, {"", 80, 1}}; !slices.Equal(cs.Labels, want) {
		t.Errorf("labels = %v, want %v", cs.Labels, want)
	}
}
