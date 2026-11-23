package taxguess

import (
	"errors"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/payroll"
)

var france = In{Country: "fr", Name: "France", Currency: "EUR", Year: 2026}

func TestParse(t *testing.T) {
	r, err := Parse("```json\n"+`{"found":true,"year":2026,"currency":"eur","region":"","brackets":[{"up_to":11600,"rate":0},{"up_to":29600,"rate":11},{"up_to":0,"rate":0.3}],
		"allowance":{"amount":0,"taper_from":0,"taper_rate":0,"as_credit":false},"deductions":[{"name":"Professional expenses","amount":500}],
		"contributions":[{"name":" CSG ","rate":9.2,"floor":0,"cap":0,"monthly":false,"deductible":false}],"pays_per_year":12,"notes":" Simplified. "}`+"\n```", france)
	if err != nil {
		t.Fatal(err)
	}
	if r.Currency != "EUR" || r.Country != "FR" || r.Source != payroll.SourceAI || r.Notes != "Simplified." {
		t.Errorf("rules = %+v", r)
	}
	if r.Brackets[1].Rate != 0.11 || r.Contributions[0].Rate != 0.092 || r.Contributions[0].Name != "CSG" {
		t.Errorf("percents not fractions: %+v %+v", r.Brackets, r.Contributions)
	}

	for _, bad := range []string{
		"nope",
		`{"found":false,"currency":"EUR","brackets":[{"up_to":0,"rate":0}]}`,
		`{"found":true,"currency":"EUR","brackets":[]}`,
		`{"found":true,"currency":"EUR","brackets":[{"up_to":20000,"rate":0.1},{"up_to":10000,"rate":0.2}]}`,
	} {
		if _, err := Parse(bad, france); !errors.Is(err, ErrNoRules) {
			t.Errorf("Parse(%q) err = %v", bad, err)
		}
	}
}

func TestKeyAndPrompt(t *testing.T) {
	if Key("fr", " Île-de-France ", 2026) != "v1|FR|ile de france|2026" {
		t.Errorf("key = %q", Key("fr", " Île-de-France ", 2026))
	}
	in := france
	in.Region = "Bavaria"
	in.Name, in.Country = "Germany", "DE"
	p := Prompt(in)
	for _, want := range []string{"Bavaria, Germany", "2026", "EUR", "EMPLOYEE"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
}
