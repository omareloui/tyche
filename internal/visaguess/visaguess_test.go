package visaguess

import (
	"errors"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/visa"
)

var spain = In{Country: "es", Name: "Spain", Currency: "EUR", Visa: "nomad", Year: 2026}

func TestParse(t *testing.T) {
	r, err := Parse("```json\n"+`{"found":true,"visa_name":" Digital Nomad Visa ","category":"nomad","year":2026,"currency":"eur",
		"requirements":[{"kind":"income","period":"month","amount":2849,"per_dependent":[1068,356],"group":"","months_proof":3,"text":" Income "},
		{"kind":"check","period":"once","amount":5,"per_dependent":[0],"group":"","months_proof":2,"text":"Remote work"}],"notes":" Varies. "}`+"\n```", spain)
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "Digital Nomad Visa" || r.Key != "digital-nomad-visa" || r.Currency != "EUR" || r.Country != "ES" || r.Source != visa.SourceAI || r.Notes != "Varies." {
		t.Errorf("rule = %+v", r)
	}
	if q := r.Requirements[1]; q.Amount != 0 || q.MonthsProof != 0 || q.PerDependent != nil {
		t.Errorf("check kept money: %+v", q)
	}
	if r.Requirements[0].MonthsProof != 3 || r.Requirements[0].Text != "Income" {
		t.Errorf("income = %+v", r.Requirements[0])
	}

	for _, bad := range []string{
		"nope",
		`{"found":false,"visa_name":"x","currency":"EUR","requirements":[{"kind":"check","text":"x"}]}`,
		`{"found":true,"visa_name":"x","currency":"EUR","requirements":[]}`,
		`{"found":true,"visa_name":"x","currency":"EUR","requirements":[{"kind":"income","period":"month","amount":-5}]}`,
		`{"found":true,"visa_name":"x","currency":"EUR","requirements":[{"kind":"lottery","period":"once","amount":5}]}`,
		`{"found":true,"visa_name":"x","currency":"EUR","requirements":[{"kind":"savings","period":"once","amount":0,"text":""}]}`,
	} {
		if _, err := Parse(bad, spain); !errors.Is(err, ErrNoRules) {
			t.Errorf("Parse(%q) err = %v", bad, err)
		}
	}
}

func TestParseRepairs(t *testing.T) {
	r, err := Parse(`{"found":true,"visa_name":"Golden Visa","category":"investor","year":2026,"currency":"EUR","requirements":[
		{"kind":"investment","period":"month","amount":250000,"per_dependent":[-1],"group":"","months_proof":3,"text":""},
		{"kind":"savings","period":"once","amount":0,"per_dependent":[],"group":"","months_proof":0,"text":"Sufficient funds for the stay"},
		{"kind":"Income","period":"weekly","amount":3000,"per_dependent":[],"group":"","months_proof":99,"text":""}],"notes":""}`, spain)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Requirements) != 3 {
		t.Fatalf("requirements = %+v", r.Requirements)
	}
	if q := r.Requirements[0]; q.Period != visa.PeriodOnce || q.PerDependent != nil || q.MonthsProof != 0 {
		t.Errorf("investment = %+v", q)
	}
	if q := r.Requirements[1]; q.Kind != visa.KindCheck || q.Text != "Sufficient funds for the stay" {
		t.Errorf("amount-less savings = %+v", q)
	}
	if q := r.Requirements[2]; q.Kind != visa.KindIncome || q.Period != visa.PeriodMonth || q.MonthsProof != 0 {
		t.Errorf("income = %+v", q)
	}
}

func TestKeyAndPrompt(t *testing.T) {
	if got := Key("es", " Visado No Lucrativo ", 2026); got != "v1|ES|visado no lucrativo|2026" {
		t.Errorf("key = %q", got)
	}
	p := Prompt(spain)
	for _, want := range []string{`"nomad"`, "Spain", "2026", "EUR"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
}
