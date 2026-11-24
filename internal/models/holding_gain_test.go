package models

import (
	"math"
	"testing"
	"time"
)

func hgDay(d int) time.Time { return time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC) }

// hgRates are USD's and XAU's rates to EGP by day, today's at 0.
type hgRates map[string]map[int]float64

func (r hgRates) on(day time.Time, code string) (float64, bool) {
	v, ok := r[code][day.Day()]
	if !ok {
		return r[code][0], false
	}
	return v, true
}

func (r hgRates) today(code string) float64 { return r[code][0] }

func hgNear(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestHoldingGainsBuySell(t *testing.T) {
	rates := hgRates{"USD": {0: 50, 1: 48, 2: 49, 3: 52}}
	accs := []HoldingAccount{{ID: "usd", Code: "USD", Balance: 150, Opened: hgDay(1)}}
	moves := []HoldingMove{
		// 100 USD for 4,800 EGP, then 100 for 5,000 (49 + fees), then 50
		// sold for 2,600.
		{Day: hgDay(1), Kind: TransferTxKind, AccountID: "egp", ToAccountID: "usd", Amount: 4800, ToAmount: 100, FromCode: "EGP", ToCode: "USD"},
		{Day: hgDay(2), Kind: TransferTxKind, AccountID: "egp", ToAccountID: "usd", Amount: 5000, ToAmount: 100, FromCode: "EGP", ToCode: "USD"},
		{Day: hgDay(3), Kind: TransferTxKind, AccountID: "usd", ToAccountID: "egp", Amount: 50, ToAmount: 2600, FromCode: "USD", ToCode: "EGP"},
	}
	g := ComputeHoldingGains("EGP", accs, moves, rates.on, rates.today)["usd"]
	if !hgNear(g.Units, 150) || !hgNear(g.AvgCost(), 49) || !hgNear(g.Basis, 7350) {
		t.Fatalf("units %v avg %v basis %v", g.Units, g.AvgCost(), g.Basis)
	}
	if !hgNear(g.Realized, 2600-50*49) {
		t.Errorf("realized = %v", g.Realized)
	}
	if !hgNear(g.Value, 7500) || !hgNear(g.Unrealized, 150) || !hgNear(g.UnrealizedPct(), 150.0/7350*100) {
		t.Errorf("value %v unrealized %v pct %v", g.Value, g.Unrealized, g.UnrealizedPct())
	}
	if g.Estimated {
		t.Error("estimated without a guess")
	}
}

func TestHoldingGainsOpeningEstimated(t *testing.T) {
	rates := hgRates{"USD": {0: 50, 5: 47}}
	// Created with 200, then 20 spent: 180 left.
	accs := []HoldingAccount{{ID: "usd", Code: "USD", Balance: 180, Opened: hgDay(5)}}
	moves := []HoldingMove{{Day: hgDay(6), Kind: ExpenseTxKind, AccountID: "usd", Amount: 20, FromCode: "USD"}}
	g := ComputeHoldingGains("EGP", accs, moves, rates.on, rates.today)["usd"]
	if !g.Estimated {
		t.Error("opening balance not estimated")
	}
	if !hgNear(g.Units, 180) || !hgNear(g.AvgCost(), 47) {
		t.Errorf("units %v avg %v", g.Units, g.AvgCost())
	}
	// Day 6's rate is missing: today's, so 20 × 50 − 20 × 47.
	if !hgNear(g.Realized, 60) {
		t.Errorf("realized = %v", g.Realized)
	}
}

func TestHoldingGainsFeeAndCarry(t *testing.T) {
	rates := hgRates{"USD": {0: 50}}
	accs := []HoldingAccount{
		{ID: "usd", Code: "USD", Balance: 40, Opened: hgDay(1)},
		{ID: "goal", Code: "USD", Balance: 59, Opened: hgDay(1)},
	}
	moves := []HoldingMove{
		// 100 USD cost 4,900 EGP, fees included.
		{Day: hgDay(1), Kind: TransferTxKind, AccountID: "egp", ToAccountID: "usd", Amount: 4900, ToAmount: 100, FromCode: "EGP", ToCode: "USD"},
		// 60 to a goal's holding, 1 USD fee: the cost of 60 moves along.
		{Day: hgDay(2), Kind: TransferTxKind, AccountID: "usd", ToAccountID: "goal", Amount: 60, ToAmount: 59, FromCode: "USD", ToCode: "USD"},
	}
	gains := ComputeHoldingGains("EGP", accs, moves, rates.on, rates.today)
	if g := gains["usd"]; !hgNear(g.Basis, 40*49) || g.Realized != 0 {
		t.Errorf("usd basis %v realized %v", g.Basis, g.Realized)
	}
	if g := gains["goal"]; !hgNear(g.Basis, 60*49) || !hgNear(g.Units, 59) || g.Realized != 0 {
		t.Errorf("goal basis %v units %v realized %v", g.Basis, g.Units, g.Realized)
	}
	if s := SumHoldingGains(gains["usd"], gains["goal"]); !hgNear(s.Basis, 4900) || !hgNear(s.Value, 99*50) {
		t.Errorf("sum basis %v value %v", s.Basis, s.Value)
	}
}

func TestHoldingGainsMetal(t *testing.T) {
	// Gold in troy ounces: 1 oz bought with USD at 2,000 USD (EGP 100,000
	// that day), worth 110,000 today.
	rates := hgRates{"XAU": {0: 110000}, "USD": {0: 50, 1: 50}}
	accs := []HoldingAccount{
		{ID: "usd", Code: "USD", Balance: 0, Opened: hgDay(1)},
		{ID: "gold", Code: "XAU", Balance: 1, Opened: hgDay(1)},
	}
	moves := []HoldingMove{
		{Day: hgDay(1), Kind: TransferTxKind, AccountID: "egp", ToAccountID: "usd", Amount: 98000, ToAmount: 2000, FromCode: "EGP", ToCode: "USD"},
		{Day: hgDay(1), Kind: TransferTxKind, AccountID: "usd", ToAccountID: "gold", Amount: 2000, ToAmount: 1, FromCode: "USD", ToCode: "XAU"},
	}
	gains := ComputeHoldingGains("EGP", accs, moves, rates.on, rates.today)
	g := gains["gold"]
	if !hgNear(g.Basis, 100000) || !hgNear(g.Unrealized, 10000) || !hgNear(g.Units, 1) {
		t.Errorf("gold basis %v unrealized %v units %v", g.Basis, g.Unrealized, g.Units)
	}
	if u := gains["usd"]; !hgNear(u.Realized, 2000) || u.Units != 0 {
		t.Errorf("usd realized %v units %v", u.Realized, u.Units)
	}
}

func TestHoldingGainsTransferPair(t *testing.T) {
	rates := hgRates{"USD": {0: 50}}
	accs := []HoldingAccount{{ID: "usd", Code: "USD", Balance: 100, Opened: hgDay(1)}}
	moves := []HoldingMove{
		// Two SMS of one transfer: the income's pair is 4,850 EGP out.
		{Day: hgDay(1), Kind: IncomeTxKind, AccountID: "usd", Amount: 100, FromCode: "USD", PairAmount: 4850, PairCode: "EGP"},
	}
	rateOn := func(time.Time, string) (float64, bool) { t.Fatal("asked a rate"); return 0, false }
	g := ComputeHoldingGains("EGP", accs, moves, rateOn, rates.today)["usd"]
	if !hgNear(g.Basis, 4850) || g.Estimated {
		t.Errorf("basis %v estimated %v", g.Basis, g.Estimated)
	}
	if days, codes := HoldingRateNeeds("EGP", accs, moves); len(days) != 0 || len(codes) != 0 {
		t.Errorf("needs %v %v", days, codes)
	}
}
