package payroll

import (
	"errors"
	"math"
	"testing"
)

func near(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.01 {
		t.Errorf("%s = %.2f, want %.2f", what, got, want)
	}
}

func rules(t *testing.T, country string, opts map[string]string) *Rules {
	t.Helper()
	r, ok := Builtin(country, opts)
	if !ok {
		t.Fatalf("no rules for %s", country)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("%s: %v", country, err)
	}
	return r
}

func TestBuiltinsValidate(t *testing.T) {
	for _, c := range BuiltinCountries() {
		opts := BuiltinOptions(c)
		if len(opts) == 0 {
			rules(t, c, nil)
			continue
		}
		for _, ch := range opts[0].Choices {
			rules(t, c, map[string]string{opts[0].Key: ch.Value})
		}
	}
}

func TestCompute(t *testing.T) {
	tests := []struct {
		name         string
		country      string
		opts         map[string]string
		gross        float64
		tax, contrib float64
		net          float64
	}{
		// SI capped at 16,700 × 12 × 11% = 22,044, martyrs 120, taxable
		// 240,000 − 22,044 − 20,000 = 197,956.
		{"egypt", "EG", nil, 240_000, 29_341.2, 22_164, 188_494.8},
		{"uk", "GB", nil, 50_000, 7_486, 2_994.4, 39_519.6},
		// Allowance tapered to 7,570.
		{"uk taper", "GB", nil, 110_000, 33_432, 4_210.6, 72_357.4},
		{"us single", "US", map[string]string{"filing": "single"}, 100_000, 13_170, 7_650, 79_180},
		// Taxable 30,000 − 1,950 − 2,000 = 26,050: 5,980.5 less the
		// personal minimum's 1,054.5.
		{"spain", "ES", nil, 30_000, 4_926, 1_950, 23_124},
		{"saudi new", "SA", map[string]string{"nationality": "saudi_new"}, 120_000, 0, 12_900, 107_100},
		{"saudi expat", "SA", nil, 120_000, 0, 0, 120_000},
		// Ontario, $100,000: see TestCanada for the parts.
		{"canada ontario", "CA", nil, 100_000, 19_274.35, 6_519.52, 74_206.13},
		{"uae national", "AE", map[string]string{"nationality": "national"}, 240_000, 0, 26_400, 213_600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := rules(t, tt.country, tt.opts)
			res := r.Compute(Input{AnnualGross: tt.gross})
			near(t, "tax", res.Tax, tt.tax)
			near(t, "contributions", res.TotalContribs(), tt.contrib)
			near(t, "net", res.Net, tt.net)
		})
	}
}

func TestEgyptHighIncomeTable(t *testing.T) {
	r := rules(t, "EG", nil)
	r.Contributions, r.Deductions = nil, nil
	res := r.Compute(Input{AnnualGross: 650_000})
	// Over 600,000 the 0% bracket is gone.
	near(t, "tax", res.Tax, 5_500+2_250+26_000+45_000+62_500)
}

func TestScotland(t *testing.T) {
	r := rules(t, "GB", map[string]string{"region": "scotland"})
	res := r.Compute(Input{AnnualGross: 30_000})
	// 3,967 × 19% + 12,989 × 20% + 474 × 21%.
	near(t, "tax", res.Tax, 3_967*0.19+12_989*0.20+474*0.21)
}

func TestBonusAndOther(t *testing.T) {
	r := rules(t, "US", nil)
	base := r.Compute(Input{AnnualGross: 100_000})
	res := r.Compute(Input{AnnualGross: 100_000, Bonus: 10_000})
	near(t, "bonus net", res.BonusNet, res.Net-base.Net)
	// 22% + 7.65%.
	near(t, "bonus net", res.BonusNet, 10_000*(1-0.22-0.0765))
	other := r.Compute(Input{AnnualGross: 100_000, OtherRate: 0.05})
	near(t, "other", other.Other, 5_000)
	near(t, "net", other.Net, base.Net-5_000)
	near(t, "marginal", base.MarginalRate, 0.22+0.0765)
}

func TestGrossFor(t *testing.T) {
	for _, c := range BuiltinCountries() {
		r := rules(t, c, nil)
		for _, g := range []float64{30_000, 250_000, 1_500_000} {
			net := r.Compute(Input{AnnualGross: g}).Net
			got, ok := r.GrossFor(net, Input{})
			if !ok {
				t.Fatalf("%s: no gross for %.2f", c, net)
			}
			back := r.Compute(Input{AnnualGross: got}).Net
			if math.Abs(back-net) > 0.05 {
				t.Errorf("%s: gross %.2f takes home %.2f, want %.2f", c, got, back, net)
			}
		}
	}
}

func TestValidate(t *testing.T) {
	ok := Rules{Currency: "EUR", Brackets: []Bracket{{10_000, 0}, {0, 0.2}}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := []Rules{
		{Currency: "EUR"},
		{Currency: "EUR", Brackets: []Bracket{{10_000, 1.5}}},
		{Currency: "EUR", Brackets: []Bracket{{20_000, 0.1}, {10_000, 0.2}}},
		{Currency: "EUR", Brackets: []Bracket{{0, 0.1}, {10_000, 0.2}}},
		{Currency: "EU", Brackets: []Bracket{{0, 0.1}}},
		{Currency: "EUR", Brackets: []Bracket{{0, 0.1}}, Contributions: []Contribution{{Name: "x", Rate: 0.1, Floor: 10, Cap: 5}}},
		{Currency: "EUR", Brackets: []Bracket{{0, 0.1}}, Contributions: []Contribution{{Rate: 0.1}}},
	}
	for i, r := range bad {
		if err := r.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%d: got %v, want invalid", i, err)
		}
	}
}

func TestCloneIsDeep(t *testing.T) {
	r := rules(t, "EG", nil)
	c := r.Clone()
	c.Brackets[0].Rate = 0.5
	c.Tables[0].Brackets[0].Rate = 0.5
	c.Contributions[0].Rate = 0.5
	if r.Brackets[0].Rate == 0.5 || r.Tables[0].Brackets[0].Rate == 0.5 || r.Contributions[0].Rate == 0.5 {
		t.Fatal("clone shares slices")
	}
}

func TestTextRoundTrip(t *testing.T) {
	r := rules(t, "EG", nil)
	tables, err := ParseTables(FormatTables(r.Tables))
	if err != nil {
		t.Fatal(err)
	}
	if FormatTables(tables) != FormatTables(r.Tables) || tables[0].Brackets[0].Rate != 0.10 {
		t.Errorf("tables = %v", FormatTables(tables))
	}
	if got := FormatBands([]Bracket{{12_570, 0}, {50_270, 0.08}, {0, 0.02}}); got != "12570:0 50270:8 *:2" {
		t.Errorf("bands = %q", got)
	}
	es := rules(t, "ES", nil)
	steps, err := ParseSteps(FormatSteps(es.WorkReduction))
	if err != nil || len(steps) != 3 || steps[1].Slope != 1.75 {
		t.Errorf("steps = %v, %v", steps, err)
	}
	for _, bad := range []string{"10000", "x:10", "100:y"} {
		if _, err := ParseBands(bad); err == nil {
			t.Errorf("ParseBands(%q) took it", bad)
		}
	}
	if _, err := ParseTables("100 10:5"); err == nil {
		t.Error("table without >")
	}
	if _, err := ParseSteps("1 2"); err == nil {
		t.Error("short step")
	}
}

func TestBonusRateAndGrossFor(t *testing.T) {
	r := rules(t, "US", nil)
	byRate := r.Compute(Input{AnnualGross: 100_000, BonusRate: 0.1})
	byAmount := r.Compute(Input{AnnualGross: 100_000, Bonus: 10_000})
	near(t, "bonus", byRate.Bonus, 10_000)
	near(t, "net", byRate.Net, byAmount.Net)
	// The bonus doesn't count toward the net asked for.
	g, ok := r.GrossFor(79_180, Input{BonusRate: 0.1})
	if !ok {
		t.Fatal("no gross")
	}
	near(t, "gross", g, 100_000)
}

func TestCanada(t *testing.T) {
	r := rules(t, "CA", nil)
	res := r.Compute(Input{AnnualGross: 100_000})
	// Taxable 100,000 − 711 (enhanced CPP) − 416 (CPP2) = 98,873.
	near(t, "taxable", res.Taxable, 98_873)
	// Federal: 16,464.97 less 14% of 16,452 + 1,501 + 3,519.45 + 1,123.07.
	near(t, "federal", res.MainTax(), 16_464.97-22_595.52*0.14)
	if len(res.Layers) != 1 || res.Layers[0].Name != "Ontario" {
		t.Fatalf("layers = %+v", res.Layers)
	}
	on := res.Layers[0]
	// 6,837.35 less 5.05% of 17,631.52, then 20% over 5,818.
	basic := 6_837.3533 - 17_631.52*0.0505
	near(t, "ontario surtax", on.Surtax, (basic-5_818)*0.20)
	near(t, "ontario", on.Tax, basic+(basic-5_818)*0.20)

	// The basic personal amount clawed back to its floor.
	high := r.Compute(Input{AnnualGross: 400_000})
	low := r.Compute(Input{AnnualGross: 100_000})
	if high.Credit >= low.Credit {
		t.Errorf("credit %.2f at 400k, %.2f at 100k", high.Credit, low.Credit)
	}

	ab := rules(t, "CA", map[string]string{"province": "ab"})
	if res := ab.Compute(Input{AnnualGross: 100_000}); res.Layers[0].Name != "Alberta" || res.Net <= low.Net {
		t.Errorf("alberta = %+v", res.Layers)
	}
}
