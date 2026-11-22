package colguess

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

func TestParse(t *testing.T) {
	e, err := Parse("```json\n" + `{"overall":2.5,"groups":{"food":1.8,"housing":4,"transport":0,"misc":99},"rent_home":15000,"rent_target":1200,"notes":" Rent. "}` + "\n```")
	if err != nil {
		t.Fatal(err)
	}
	if e.Overall != 2.5 || e.Groups[models.ColHousing] != 4 || e.Notes != "Rent." {
		t.Errorf("estimate = %+v", e)
	}
	if _, ok := e.Groups[models.ColTransport]; ok {
		t.Errorf("0 kept")
	}
	if _, ok := e.Groups[models.ColMisc]; ok {
		t.Errorf("99 kept")
	}

	for _, bad := range []string{"nope", `{"overall":0}`, `{"overall":-1}`} {
		if _, err := Parse(bad); !errors.Is(err, ErrNoEstimate) {
			t.Errorf("Parse(%q) err = %v", bad, err)
		}
	}
}

func TestKeyAndPrompt(t *testing.T) {
	in := In{
		Home:   City{Name: "  Cairo ", Country: "Egypt", Code: "eg", Currency: "EGP"},
		Target: City{Name: "Berlin", Country: "Germany", Code: "DE", Currency: "EUR"},
	}
	now := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	if k := in.Key(now); k != "v3|cairo,EG|berlin,DE|2026-11" {
		t.Errorf("key = %q", k)
	}
	if Key("CAIRO", "EG", "berlin.", "de", now) != in.Key(now) {
		t.Errorf("same cities, other keys")
	}
	p := Prompt(in)
	for _, want := range []string{"Berlin", "Cairo", "housing", "EUR", "EGP", "home_city"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
}

func TestNormalizeCity(t *testing.T) {
	cases := map[string]string{
		"Düsseldorf":       "dusseldorf",
		"  São   Paulo ":   "sao paulo",
		"St. Louis":        "st louis",
		"Saint-Étienne":    "saint etienne",
		"Washington, D.C.": "washington d c",
		"القاهرة":          "القاهرة",
		"Ōsaka":            "osaka",
	}
	for in, want := range cases {
		if got := NormalizeCity(in); got != want {
			t.Errorf("NormalizeCity(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseCities(t *testing.T) {
	e, err := Parse(`{"home_city":"  Cairo ","target_city":"New York City","overall":3,"groups":{}}`)
	if err != nil {
		t.Fatal(err)
	}
	if e.HomeCity != "Cairo" || e.TargetCity != "New York City" {
		t.Errorf("cities = %q %q", e.HomeCity, e.TargetCity)
	}
}

func TestSwapped(t *testing.T) {
	in := In{Home: City{Name: "Cairo"}, Target: City{Name: "madrid"}}
	cases := []struct {
		home, target string
		want         bool
	}{
		{"Cairo", "Madrid", false},
		{"Madrid", "Cairo", true},
		{"", "Cairo", false},
		{"Giza", "Madrid", false},
	}
	for _, c := range cases {
		if got := Swapped(in, &models.ColCityEstimate{HomeCity: c.home, TargetCity: c.target}); got != c.want {
			t.Errorf("Swapped(%q, %q) = %v", c.home, c.target, got)
		}
	}
}

func TestPromptReference(t *testing.T) {
	in := In{
		Home:   City{Name: "Cairo", Country: "Egypt", Code: "EG", Currency: "EGP"},
		Target: City{Name: "Madrid", Country: "Spain", Code: "ES", Currency: "EUR"},
	}
	if strings.Contains(Prompt(in), "World Bank") {
		t.Errorf("reference without one")
	}
	// Ratios in EUR per EGP at a market rate of 0.02: levels 4 and 2.5.
	r := &models.ColRatios{Avg: 0.08, Groups: map[models.ColGroup]float64{models.ColFood: 0.05}}
	in.Ref = NewReference(r, 0.02, 2021)
	p := Prompt(in)
	for _, want := range []string{"World Bank", "2021", "overall: 4.00", "food: 2.50"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q:\n%s", want, p)
		}
	}
	if NewReference(nil, 0.02, 2021) != nil || NewReference(r, 0, 2021) != nil {
		t.Errorf("reference without ratios or rate")
	}
}
