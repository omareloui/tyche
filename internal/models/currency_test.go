package models

import (
	"math"
	"testing"
)

func TestCurrencyFormatMetalUnits(t *testing.T) {
	usd := Currency{String: "USD", Symbol: "US$"}
	xau := Currency{String: "XAU", Symbol: "XAU"}
	xauG := Currency{String: "XAU", Symbol: "XAU", InGrams: true}
	// Joined rows only carry the symbol.
	xagSym := Currency{Symbol: "XAG", InGrams: true}

	tests := []struct {
		name   string
		cur    Currency
		amount float64
		want   string
	}{
		{"fiat", usd, 1234.5, "US$ 1,234.5"},
		{"fiat in grams is ignored", Currency{String: "USD", Symbol: "US$", InGrams: true}, 10, "US$ 10"},
		{"troy ounces", xau, 1.23456, "XAU 1.2346 oz"},
		{"grams", xauG, 1, "XAU 31.1 g"},
		{"grams by symbol", xagSym, 10 / GramsPerTroyOunce, "XAG 10 g"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cur.Format(tt.amount); got != tt.want {
				t.Errorf("Format(%v) = %q, want %q", tt.amount, got, tt.want)
			}
		})
	}
}

func TestMetalUnitRoundTrip(t *testing.T) {
	xau := Currency{String: "XAU", Symbol: "XAU", Kind: CurrencyKindPreciousMetal}

	// 10 g typed in, stored in troy ounces then shown back in grams.
	stored := xau.Round(FromUnit(10, MetalUnitGram))
	if got := InputValue(stored, MetalUnitGram); got != "10.00" {
		t.Errorf("InputValue in grams = %q, want 10.00", got)
	}
	if got := InputValue(stored, MetalUnitOunce); got != "0.3215" {
		t.Errorf("InputValue in ounces = %q, want 0.3215", got)
	}
	if got := ToUnit(stored, MetalUnitGram); math.Abs(got-10) > 1e-4 {
		t.Errorf("ToUnit = %v, want ~10", got)
	}

	if got := (&Currency{String: "EGP"}).Round(1.23456); got != 1.23 {
		t.Errorf("fiat Round = %v, want 1.23", got)
	}
}

func TestAccountAmountUnit(t *testing.T) {
	gold := &Account{Currency: Currency{String: "XAU", InGrams: true}}
	cash := &Account{Currency: Currency{String: "EGP"}}

	if got := gold.AmountUnit(""); got != MetalUnitGram {
		t.Errorf("default unit = %q, want g", got)
	}
	if got := gold.AmountUnit("oz"); got != MetalUnitOunce {
		t.Errorf("picked unit = %q, want oz", got)
	}
	if got := gold.AmountUnit("kg"); got != MetalUnitGram {
		t.Errorf("bad picked unit = %q, want g", got)
	}
	if got := cash.AmountUnit("g"); got != "" {
		t.Errorf("fiat unit = %q, want none", got)
	}
	if got := (*Account)(nil).AmountUnit("g"); got != "" {
		t.Errorf("nil account unit = %q, want none", got)
	}
}
