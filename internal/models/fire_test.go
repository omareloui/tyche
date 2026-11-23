package models

import (
	"math"
	"testing"
)

func fireIn() FireInput {
	return FireInput{Portfolio: 100_000, Spending: 2_000, Saving: 1_000, Return: 0.05, SWR: 0.04, Volatility: 0.15, Age: 30, RetireAge: 65}
}

func TestFireNumber(t *testing.T) {
	if n := FireNumber(2_000, 0.04); n != 600_000 {
		t.Fatalf("number = %v, want 600000", n)
	}
	if n := FireNumber(2_000, 0); n != 0 {
		t.Fatalf("number at 0%% = %v", n)
	}
}

func TestComputeFire(t *testing.T) {
	res := ComputeFire(fireIn())
	if res.Number != 600_000 || res.Never {
		t.Fatalf("number %v never %v", res.Number, res.Never)
	}
	// Check the months against a plain loop.
	p, mr := 100_000.0, math.Pow(1.05, 1.0/12)-1
	m := 0
	for p < 600_000 {
		p = p*(1+mr) + 1_000
		m++
	}
	if res.Months != m {
		t.Fatalf("months = %d, want %d", res.Months, m)
	}
	if res.Age != 30+m/12 {
		t.Fatalf("age = %d", res.Age)
	}
	if math.Abs(res.Progress-1.0/6) > 1e-9 {
		t.Fatalf("progress = %v", res.Progress)
	}
	if got := res.Path[0].Portfolio; got != 100_000 {
		t.Fatalf("path starts at %v", got)
	}
	if !res.Path[len(res.Path)-1].Retired {
		t.Fatal("the path ends retired")
	}

	// No saving and no return: never.
	in := fireIn()
	in.Saving, in.Return = 0, 0
	if res := ComputeFire(in); !res.Never {
		t.Fatal("no saving should never get there")
	}
	// In debt: saving pays it off, then grows.
	in = fireIn()
	in.Portfolio = -20_000
	if res := ComputeFire(in); res.Never || res.Progress != 0 || res.Months <= ComputeFire(fireIn()).Months {
		t.Fatalf("in debt: never %v progress %v months %d", res.Never, res.Progress, res.Months)
	}
	if err := in.Validate(); err != nil {
		t.Fatalf("a negative portfolio is valid: %v", err)
	}
	// Already there.
	in = fireIn()
	in.Portfolio = 700_000
	if res := ComputeFire(in); res.Months != 0 || res.Never || res.Progress != 1 {
		t.Fatalf("already there: %+v", res.Months)
	}
}

func TestFireVariants(t *testing.T) {
	in := fireIn()
	in.PartTime = 500
	res := ComputeFire(in)
	byKey := map[string]FireVariant{}
	for _, v := range res.Variants {
		byKey[v.Key] = v
	}
	if v := byKey["lean"]; v.Number != 450_000 || v.Months >= res.Months {
		t.Fatalf("lean %+v", v)
	}
	if v := byKey["fat"]; v.Number != 900_000 || v.Months <= res.Months {
		t.Fatalf("fat %+v", v)
	}
	if v := byKey["barista"]; v.Number != 450_000 {
		t.Fatalf("barista %+v", v)
	}
	coast, ok := byKey["coast"]
	if !ok {
		t.Fatal("no coast")
	}
	if want := 600_000 / math.Pow(1.05, 35); math.Abs(coast.Number-want) > 1e-6 {
		t.Fatalf("coast number %v, want %v", coast.Number, want)
	}
	// 100k is above 600k / 1.05^35 ≈ 109k? It isn't: saving gets there soon.
	if coast.Never || coast.Months == 0 || coast.Months > 24 {
		t.Fatalf("coast months %d", coast.Months)
	}

	// Without an age, no coast; without part-time, no barista.
	in = fireIn()
	in.Age = 0
	for _, v := range ComputeFire(in).Variants {
		if v.Key == "coast" || v.Key == "barista" {
			t.Fatalf("unexpected %s", v.Key)
		}
	}
}

func TestFireValidate(t *testing.T) {
	bad := []func(*FireInput){
		func(in *FireInput) { in.Spending = 0 },
		func(in *FireInput) { in.SWR = 0.2 },
		func(in *FireInput) { in.Return = 0.5 },
		func(in *FireInput) { in.Volatility = 1 },
		func(in *FireInput) { in.RetireAge = 25 },
		func(in *FireInput) { in.Saving = -1 },
		func(in *FireInput) { in.PartTime = -1 },
	}
	if err := fireIn().Validate(); err != nil {
		t.Fatal(err)
	}
	for i, f := range bad {
		in := fireIn()
		f(&in)
		if in.Validate() == nil {
			t.Errorf("case %d passed", i)
		}
	}
}

func TestSimulateFire(t *testing.T) {
	in := fireIn()
	years := fireChartYears(ComputeFire(in).Months, false)
	a := SimulateFire(in, 500, years, FireSeed(in))
	b := SimulateFire(in, 500, years, FireSeed(in))
	if a.SuccessRate != b.SuccessRate || a.Years != b.Years || a.Bands[years].P50 != b.Bands[years].P50 {
		t.Fatal("the same seed should give the same result")
	}
	if len(a.Bands) != years+1 {
		t.Fatalf("bands %d", len(a.Bands))
	}
	for _, band := range a.Bands {
		if band.P10 > band.P50 || band.P50 > band.P90 {
			t.Fatalf("band out of order %+v", band)
		}
	}
	if a.SuccessRate <= 0 || a.SuccessRate > 1 || a.ReachRate <= 0.9 {
		t.Fatalf("rates %v %v", a.SuccessRate, a.ReachRate)
	}
	if !(a.Years[0] <= a.Years[1] && a.Years[1] <= a.Years[2]) {
		t.Fatalf("years %v", a.Years)
	}

	// No volatility: every run is the same, and 4% at a 5% return lasts.
	in.Volatility = 0
	s := SimulateFire(in, 50, years, 1)
	if s.SuccessRate != 1 || s.ReachRate != 1 || s.Years[0] != s.Years[2] {
		t.Fatalf("flat sim %+v", s)
	}
	// Spending everything at once in a 0% world doesn't last.
	in.Return, in.SWR = 0, 0.10
	if s := SimulateFire(in, 50, years, 1); s.SuccessRate != 0 {
		t.Fatalf("10%% at 0%% lasted: %v", s.SuccessRate)
	}
	// Never reaching it.
	in = fireIn()
	in.Saving, in.Return, in.Volatility = 0, 0, 0
	if s := SimulateFire(in, 20, FireChartYears, 1); s.ReachRate != 0 || s.Years[1] != -1 {
		t.Fatalf("never %+v", s.Years)
	}
}
