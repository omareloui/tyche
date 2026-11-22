package models

import (
	"math"
	"net/url"
	"testing"
	"time"
)

func TestItemKey(t *testing.T) {
	for in, want := range map[string]string{
		"MILK 1L":         "milk 1l",
		"  Milk \t 1l. ":  "milk 1l",
		"*Bread*":         "bread",
		"Coffee - Large:": "coffee - large",
		"...":             "",
	} {
		if got := ItemKey(in); got != want {
			t.Errorf("ItemKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatchItem(t *testing.T) {
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cands := []ItemCandidate{
		{ID: "alias-old", NameKey: "whole milk", Aliases: []string{"MILK 1L"}, CreatedAt: old},
		{ID: "name", NameKey: "milk 1l", CreatedAt: old.AddDate(0, 1, 0)},
		{ID: "bread", NameKey: "bread", CreatedAt: old},
	}
	if c := MatchItem("Milk 1l", cands); c == nil || c.ID != "name" {
		t.Errorf("a name beats an alias, got %+v", c)
	}
	cands[0].AtMerchant = true
	if c := MatchItem("Milk 1l", cands); c == nil || c.ID != "alias-old" {
		t.Errorf("the merchant's item first, got %+v", c)
	}
	if c := MatchItem("cheese", cands); c != nil {
		t.Errorf("no match, got %+v", c)
	}
	if c := MatchItem("  ", cands); c != nil {
		t.Errorf("blank name, got %+v", c)
	}
}

func TestItemAbsorb(t *testing.T) {
	cat := &ItemCategory{ID: "c"}
	it := Item{Name: "Milk", Aliases: []string{"MILK 1L"}}
	it.Absorb(&Item{Name: "milk 1l", Aliases: []string{"Lait", "milk"}, Category: cat})
	if want := []string{"MILK 1L", "Lait"}; !equalStrings(it.Aliases, want) {
		t.Errorf("aliases = %v, want %v", it.Aliases, want)
	}
	if it.Category != cat {
		t.Error("category not taken")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestNewItemStats(t *testing.T) {
	egp := Currency{String: "EGP"}
	xyz := Currency{String: "XYZ"} // no rate
	shop := &Counterparty{ID: "shop", Name: "Shop"}
	mart := &Counterparty{ID: "mart", Name: "Mart"}
	s := NewItemStats([]ItemLine{
		{TxID: "1", Day: day("2026-01-01"), Quantity: 2, Total: 20, Currency: egp, Merchant: shop},
		// Lines in EUR on an EGP account: the tx's rate, 500 / 10 = 50.
		{TxID: "2", Day: day("2026-02-01"), Quantity: 1, Total: 0.5, Foreign: true, LinesTotal: 10, Amount: 500, Currency: egp, Merchant: mart},
		{TxID: "3", Day: day("2026-03-01"), Quantity: 1, Total: 7, Currency: xyz, Merchant: shop},
		{TxID: "4", Day: day("2026-04-01"), Quantity: 4, Total: 48, Currency: egp, Merchant: shop},
	}, egp)

	if !s.Partial {
		t.Error("the XYZ line has no rate: partial")
	}
	if s.Times != 3 || s.Qty != 7 || s.Spent != 93 {
		t.Errorf("times %d qty %v spent %v", s.Times, s.Qty, s.Spent)
	}
	if s.First != 10 || s.Last != 12 || s.Min != 10 || s.Max != 25 {
		t.Errorf("first %v last %v min %v max %v", s.First, s.Last, s.Min, s.Max)
	}
	if math.Abs(s.Avg-13.29) > 0.001 {
		t.Errorf("avg = %v", s.Avg)
	}
	if math.Abs(s.ChangePct()-20) > 0.001 {
		t.Errorf("change = %v", s.ChangePct())
	}
	if s.Purchases[0].TxID != "4" {
		t.Errorf("purchases newest first, got %s", s.Purchases[0].TxID)
	}
	if len(s.PerMerchant) != 2 || s.PerMerchant[0].Merchant != shop {
		t.Fatalf("per merchant = %+v", s.PerMerchant)
	}
	if m := s.PerMerchant[0]; m.Times != 2 || m.Min != 10 || m.Last != 12 || m.Avg != 11.33 {
		t.Errorf("shop = %+v", m)
	}
}

func TestParseItemsRange(t *testing.T) {
	today := day("2026-11-22")
	ms := MonthStart(25) // November = Oct 25 – Nov 24
	for _, c := range []struct {
		q        string
		key      string
		from, to string
		values   string
	}{
		{"", ItemsRangeAll, "", "", ""},
		{"range=nope", ItemsRangeAll, "", "", ""},
		{"range=month", ItemsRangeMonth, "2026-10-25", "2026-11-25", "range=month"},
		{"range=last-month", ItemsRangeLastMonth, "2026-09-25", "2026-10-25", "range=last-month"},
		{"range=3m", ItemsRange3Months, "2026-08-25", "2026-11-25", "range=3m"},
		{"range=year", ItemsRangeYear, "2026-01-01", "2027-01-01", "range=year"},
		{"range=custom&from=2026-11-10&to=2026-11-01", ItemsRangeCustom, "2026-11-01", "2026-11-11", "from=2026-11-01&range=custom&to=2026-11-10"},
		{"range=custom&from=2026-11-10", ItemsRangeCustom, "2026-11-10", "", "from=2026-11-10&range=custom"},
		{"range=custom", ItemsRangeCustom, "", "", "range=custom"},
	} {
		v, _ := url.ParseQuery(c.q)
		r := ParseItemsRange(v, today, ms)
		date := func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("2006-01-02")
		}
		if r.Key != c.key || date(r.From) != c.from || date(r.To) != c.to || r.Values().Encode() != c.values {
			t.Errorf("%q = %s %s %s (%s)", c.q, r.Key, date(r.From), date(r.To), r.Values().Encode())
		}
	}
}
