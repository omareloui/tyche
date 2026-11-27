package splitguess

import (
	"reflect"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/models"
)

func TestParse(t *testing.T) {
	in := In{
		Lines: []Line{
			{ID: "burger", Name: "Burger", Quantity: 1},
			{ID: "pizza", Name: "Pizza", Quantity: 2},
			{ID: "drinks", Name: "Soda", Quantity: 3},
			{ID: "fries", Name: "Fries", Quantity: 1},
			{ID: "wings", Name: "Wings", Quantity: 4},
		},
		People: []Person{{ID: "ahmed", Name: "Ahmed"}},
		Text:   "Ahmed had the burger, Sara and me shared the pizzas, drinks for everyone",
	}
	ans := Answer{
		Shares: []AnswerShare{
			{Item: "I1", Takers: []Taker{{Who: "P1"}}},
			// units not said: spread
			{Item: "I2", Takers: []Taker{{Who: "Sara"}, {Who: "me"}}},
			{Item: "I3", Everyone: true},
			// a second answer for the burger is left out
			{Item: "I1", Takers: []Taker{{Who: "me"}}},
			// invented refs
			{Item: "I9", Takers: []Taker{{Who: "P1"}}},
			{Item: "I4", Takers: []Taker{{Who: "P7"}}},
			// units over the quantity, capped
			{Item: "I5", Takers: []Taker{{Who: "P1", Units: 3}, {Who: "sara", Units: 3}, {Who: "me", Units: 2}}},
		},
		NewPeople: []string{"sara", "Me", "P4"},
	}
	out := Parse(in, ans)

	if want := []string{"sara"}; !reflect.DeepEqual(out.NewPeople, want) {
		t.Errorf("new people = %v, want %v", out.NewPeople, want)
	}
	sara := NewRef(0)
	want := map[string]models.SplitItemShare{
		"burger": {With: []models.ItemShare{{Person: "ahmed", Units: 1}}},
		"pizza":  {With: []models.ItemShare{{Person: sara, Units: 1}}},
		"drinks": {All: true},
		"wings":  {With: []models.ItemShare{{Person: "ahmed", Units: 3}, {Person: sara, Units: 1}}},
	}
	if !reflect.DeepEqual(out.Shares, want) {
		t.Errorf("shares = %+v\nwant %+v", out.Shares, want)
	}
}

func TestParseByUnitsSpread(t *testing.T) {
	in := In{Lines: []Line{{ID: "b", Name: "Burgers", Quantity: 3}}, People: []Person{{ID: "a", Name: "Ali"}, {ID: "s", Name: "Sara Ali"}}}
	// a person said by name is the one in the split, any case
	out := Parse(in, Answer{Shares: []AnswerShare{{Item: "i1", Takers: []Taker{{Who: "ali"}, {Who: "SARA ALI"}}}}})
	want := models.SplitItemShare{With: []models.ItemShare{{Person: "a", Units: 2}, {Person: "s", Units: 1}}}
	if got := out.Shares["b"]; !reflect.DeepEqual(got, want) || len(out.NewPeople) != 0 {
		t.Errorf("shares = %+v, new %v; want %+v", got, out.NewPeople, want)
	}

	// the user alone: theirs, nobody else's
	out = Parse(in, Answer{Shares: []AnswerShare{{Item: "I1", Takers: []Taker{{Who: "me", Units: 3}}}}})
	if got := out.Shares["b"]; len(got.With) != 0 || got.All {
		t.Errorf("shares = %+v, want the user's", got)
	}
}

func TestParseNewPeopleCapped(t *testing.T) {
	in := In{Lines: []Line{{ID: "x", Name: "Cake", Quantity: 1}}}
	var takers []Taker
	for i := range MaxNewPeople + 3 {
		takers = append(takers, Taker{Who: "Person " + string(rune('A'+i))})
	}
	takers = append(takers, Taker{Who: strings.Repeat("x", MaxNameLen+1)})
	out := Parse(in, Answer{Shares: []AnswerShare{{Item: "I1", Takers: takers}}})
	if len(out.NewPeople) != MaxNewPeople || len(out.Shares["x"].With) != MaxNewPeople {
		t.Errorf("new %d, shared with %d; want %d", len(out.NewPeople), len(out.Shares["x"].With), MaxNewPeople)
	}
}

func TestPrompt(t *testing.T) {
	p := Prompt(In{
		Lines:  []Line{{ID: "id-1", Name: "Pizza", Quantity: 2}},
		People: []Person{{ID: "id-2", Name: "Sara"}},
		Text:   "Sara had a pizza",
	})
	for _, s := range []string{"I1: Pizza × 2", "P1: Sara", "Sara had a pizza"} {
		if !strings.Contains(p, s) {
			t.Errorf("prompt lacks %q", s)
		}
	}
	if strings.Contains(p, "id-1") || strings.Contains(p, "id-2") {
		t.Error("prompt has ids")
	}
}
