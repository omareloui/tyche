package models

import (
	"math"
	"testing"
)

func colNear(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestGuessColGroup(t *testing.T) {
	cases := map[string]ColGroup{
		"Groceries":         ColFood,
		"Supermarket":       ColFood,
		"Fast food":         ColRestaurants,
		"Eating out":        ColRestaurants,
		"Rent":              ColHousing,
		"Electricity":       ColHousing,
		"Transportation":    ColTransport,
		"Car":               ColTransport,
		"Personal care":     ColMisc,
		"Mobile & Internet": ColCommunication,
		"Netflix":           ColRecreation,
		"School fees":       ColEducation,
		"Pharmacy":          ColHealth,
		"Stuff":             ColAverage,
		"Parents":           ColAverage,
	}
	for name, want := range cases {
		if got := GuessColGroup(name); got != want {
			t.Errorf("GuessColGroup(%q) = %s, want %s", name, got, want)
		}
	}
}

func TestColRatios(t *testing.T) {
	home := ColPrices{Level: 10, Rel: map[ColGroup]float64{ColFood: 1.2, ColHousing: 0.5}}
	target := ColPrices{Level: 1, Rel: map[ColGroup]float64{ColFood: 1, ColHousing: 1.5}}
	r := home.Ratios(target)
	if r == nil || !colNear(r.Avg, 0.1) {
		t.Fatalf("avg = %v", r)
	}
	if !colNear(r.Groups[ColFood], 0.1/1.2) || !colNear(r.Groups[ColHousing], 0.3) {
		t.Errorf("groups = %v", r.Groups)
	}
	if !colNear(r.Group(ColTransport), 0.1) {
		t.Errorf("missing group takes the average")
	}
	if (ColPrices{}).Ratios(target) != nil {
		t.Errorf("no level, no ratios")
	}
}

func TestComputeCol(t *testing.T) {
	r := ColRatios{Avg: 0.1, Groups: map[ColGroup]float64{ColHousing: 0.3, ColFood: 0.05}}

	// Without spending: the average's.
	res := ComputeCol(1000, r, nil, 0.02)
	if res.HasBasket || !colNear(res.Mine, 100) || !colNear(res.Average, 100) || res.Diff != 0 {
		t.Fatalf("no basket = %+v", res)
	}
	if !colNear(res.PriceLevel, 5) {
		t.Errorf("price level = %v", res.PriceLevel)
	}

	// Half housing (3× the average ratio), half food (½).
	res = ComputeCol(1000, r, map[ColGroup]float64{ColHousing: 500, ColFood: 500}, 0)
	if !res.HasBasket || !colNear(res.MineRatio, 0.175) || !colNear(res.Mine, 175) {
		t.Fatalf("basket = %+v", res)
	}
	if !colNear(res.Diff, 0.75) || res.Driver != ColHousing {
		t.Errorf("diff = %v driver = %s", res.Diff, res.Driver)
	}
	if res.PriceLevel != 0 {
		t.Errorf("no market, no price level")
	}
	if g := res.Groups[0]; g.Group != ColFood || !colNear(g.There, 25) {
		// Equal shares: sorted by name.
		t.Errorf("groups = %+v", res.Groups)
	}

	// Mostly food: cheaper than the average.
	res = ComputeCol(1000, r, map[ColGroup]float64{ColFood: 900, ColAverage: 100}, 0)
	if res.Diff >= 0 || res.Driver != ColFood {
		t.Errorf("diff = %v driver = %s", res.Diff, res.Driver)
	}
}

func TestColCityEstimateRatios(t *testing.T) {
	e := &ColCityEstimate{Overall: 2, Groups: map[ColGroup]float64{ColHousing: 4}}
	r := e.Ratios(0.5)
	if r == nil || !colNear(r.Avg, 1) || !colNear(r.Groups[ColHousing], 2) {
		t.Fatalf("ratios = %+v", r)
	}
	if e.Ratios(0) != nil {
		t.Errorf("no market, no ratios")
	}
}

func TestColCityEstimateHousing(t *testing.T) {
	// Cairo → Madrid at 1 EGP = 0.0169 EUR: rents 12,500 EGP / 1,150 EUR
	// come to 5.44, housing 4.6 is within ColRentMismatch: kept.
	e := &ColCityEstimate{Overall: 3, Groups: map[ColGroup]float64{ColHousing: 4.6}, RentHome: 12500, RentTarget: 1150}
	if v, adj := e.HousingLevel(0.0169); adj || v != 4.6 {
		t.Errorf("housing = %v, %v", v, adj)
	}

	// Housing 2 against rents of 8: the geometric mean, 4.
	e = &ColCityEstimate{Overall: 3, Groups: map[ColGroup]float64{ColHousing: 2}, RentHome: 100, RentTarget: 400}
	v, adj := e.HousingLevel(0.5)
	if !adj || math.Abs(v-4) > 1e-9 {
		t.Errorf("housing = %v, %v", v, adj)
	}
	if r := e.Ratios(0.5); math.Abs(r.Groups[ColHousing]-2) > 1e-9 {
		t.Errorf("housing ratio = %v", r.Groups[ColHousing])
	}

	// Without rents or a housing level: as given.
	e = &ColCityEstimate{Overall: 3, Groups: map[ColGroup]float64{ColHousing: 2}}
	if v, adj := e.HousingLevel(0.5); adj || v != 2 {
		t.Errorf("no rents: %v, %v", v, adj)
	}
	e = &ColCityEstimate{Overall: 3, Groups: map[ColGroup]float64{}, RentHome: 100, RentTarget: 400}
	if r := e.Ratios(0.5); r.Groups[ColHousing] != 0 {
		t.Errorf("housing made up: %v", r.Groups[ColHousing])
	}
}
