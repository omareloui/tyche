package models

import (
	"math"
	"testing"
	"time"
)

func TestNewChargeStats(t *testing.T) {
	egp := Currency{ID: "egp", String: "EGP"}
	cafe := &Counterparty{ID: "cafe", Name: "Cafe"}
	shop := &Counterparty{ID: "shop", Name: "Shop"}
	day := func(m, d int) time.Time { return time.Date(2026, time.Month(m), d, 0, 0, 0, 0, time.UTC) }
	rate := func(v float64) *float64 { return &v }

	lines := []ChargeLine{
		// tx1 at the cafe: items 100, service 12 (rate shown), tip 10 (rate
		// worked out), VAT included 14%.
		{TxID: "tx1", Day: day(9, 26), Kind: TxItemService, Total: 12, Rate: rate(12), Subtotal: 100, LinesTotal: 122, Amount: 122, Currency: egp, Merchant: cafe},
		{TxID: "tx1", Day: day(9, 26), Kind: TxItemTip, Total: 10, Subtotal: 100, LinesTotal: 122, Amount: 122, Currency: egp, Merchant: cafe},
		{TxID: "tx1", Day: day(9, 26), Kind: TxItemVAT, Total: 12.28, Rate: rate(14), Included: true, Subtotal: 100, LinesTotal: 122, Amount: 122, Currency: egp, Merchant: cafe},
		// tx2 at the cafe, lines in another currency (10 a unit, the bank
		// charged 220 for 22): service 2 → 20.
		{TxID: "tx2", Day: day(11, 2), Kind: TxItemService, Total: 2, Subtotal: 20, LinesTotal: 22, Amount: 220, Foreign: true, Currency: egp, Merchant: cafe},
		// tx3 at the shop: a discount.
		{TxID: "tx3", Day: day(11, 3), Kind: TxItemDiscount, Total: -5, Subtotal: 50, LinesTotal: 45, Amount: 45, Currency: egp, Merchant: shop},
	}
	s := NewChargeStats(lines, egp, MonthStart(25))

	if s.Total != 37 {
		t.Errorf("Total = %v, want 37 (12+10+20-5)", s.Total)
	}
	if s.Receipts != 3 || s.ReceiptsSpent != 122+220+45 {
		t.Errorf("Receipts = %d / %v", s.Receipts, s.ReceiptsSpent)
	}
	if len(s.Included) != 1 || s.Included[0].Kind != TxItemVAT || s.Included[0].Rate != 14 {
		t.Errorf("Included = %+v", s.Included)
	}
	svc := s.Kind(TxItemService)
	if svc == nil || svc.Total != 32 || svc.Count != 2 {
		t.Fatalf("service = %+v", svc)
	}
	// 12% of 100 and 10% of 200 (20 × 10), weighted: 32 / 300.
	if want := math.Round(3200.0/300*10) / 10; svc.Rate != want {
		t.Errorf("service rate = %v, want %v", svc.Rate, want)
	}
	if tip := s.Kind(TxItemTip); tip == nil || tip.Rate != 10 {
		t.Errorf("tip = %+v, want a 10%% rate worked out", tip)
	}
	if d := s.Kind(TxItemDiscount); d == nil || d.Total != -5 || d.HasRate() {
		t.Errorf("discount = %+v", d)
	}
	if s.Kinds[0].Kind != TxItemService {
		t.Errorf("biggest first, got %v", s.Kinds[0].Kind)
	}
	if len(s.Merchants) != 2 || s.Merchants[0].Merchant != cafe || s.Merchants[0].Total != 42 || s.Merchants[0].Receipts != 2 {
		t.Errorf("merchants = %+v", s.Merchants)
	}

	// Months starting on the 25th: Sep 26 is October's, Nov 2-3 November's.
	if len(s.Months) != 2 {
		t.Fatalf("months = %+v", s.Months)
	}
	if s.Months[0].Period.Key() != "2026-10" || s.Months[0].Total != 22 {
		t.Errorf("first month = %+v", s.Months[0])
	}
	if s.Months[1].Period.Key() != "2026-11" || s.Months[1].Total != 15 {
		t.Errorf("second month = %+v", s.Months[1])
	}
}

func TestNewChargeStatsGapsAndRates(t *testing.T) {
	egp := Currency{ID: "egp", String: "EGP"}
	usd := Currency{ID: "usd", String: "USD"} // no rate
	lines := []ChargeLine{
		{TxID: "a", Day: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), Kind: TxItemFee, Total: 3, Amount: 3, LinesTotal: 3, Currency: egp},
		{TxID: "b", Day: time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC), Kind: TxItemFee, Total: 4, Amount: 4, LinesTotal: 4, Currency: egp},
		{TxID: "c", Day: time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC), Kind: TxItemFee, Total: 4, Amount: 4, LinesTotal: 4, Currency: usd},
	}
	s := NewChargeStats(lines, egp, MonthStart(1))
	if !s.Partial {
		t.Error("a line without a rate should make it partial")
	}
	if len(s.Months) != 3 || s.Months[1].Total != 0 || len(s.Months[1].Kinds) != 0 {
		t.Errorf("months = %+v, want February empty in between", s.Months)
	}
	if f := s.Kind(TxItemFee); f == nil || f.HasRate() {
		t.Errorf("a fee has no rate unless shown: %+v", f)
	}
	if (&ChargeStats{}).Empty() != true || s.Empty() {
		t.Error("Empty")
	}
}
