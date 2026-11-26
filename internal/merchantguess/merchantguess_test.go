package merchantguess

import (
	"reflect"
	"strings"
	"testing"
)

func testIn() In {
	return In{
		Names: []string{"AMZN MKTP EG*4821", "CARREFOUR MAADI 0023", "UBER *TRIP", "PAYPRO-*NEW SHOP", "SPINNEYS 12"},
		Merchants: []Merchant{
			{ID: "cp-amazon", Name: "Amazon", Aliases: []string{"AMAZON.EG"}},
			{ID: "cp-carrefour", Name: "Carrefour"},
		},
		Categories: []Category{{ID: "cat-food", Name: "Groceries"}, {ID: "cat-transport", Name: "Transport"}},
	}
}

func TestParse(t *testing.T) {
	in := testIn()
	got := Parse(in, []Answer{
		{Raw: "AMZN MKTP EG*4821", MatchRef: "M1"},
		// the same string again: the first decision holds
		{Raw: "amzn mktp eg*4821", MatchRef: "M2"},
		// named like a merchant: a match
		{Raw: "CARREFOUR MAADI 0023", NewName: "carrefour"},
		{Raw: "UBER *TRIP", NewName: "  Uber ", CategoryRef: "c2", Labels: []string{"Transport", "transport", " ", "Rides", "Taxi", "Fourth"}},
		// a made up category ref: no category
		{Raw: "PAYPRO-*NEW SHOP", NewName: "New Shop", CategoryRef: "C9"},
		// a made up merchant ref, no name: dropped
		{Raw: "SPINNEYS 12", MatchRef: "M7"},
		// never asked
		{Raw: "SOMETHING ELSE", NewName: "Else"},
	})
	want := []Decision{
		{Raw: "AMZN MKTP EG*4821", MatchID: "cp-amazon"},
		{Raw: "CARREFOUR MAADI 0023", MatchID: "cp-carrefour"},
		{Raw: "UBER *TRIP", NewName: "Uber", CategoryID: "cat-transport", Labels: []string{"Transport", "Rides", "Taxi"}},
		{Raw: "PAYPRO-*NEW SHOP", NewName: "New Shop"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse =\n%+v\nwant\n%+v", got, want)
	}
}

func TestParseAliasMatch(t *testing.T) {
	got := Parse(testIn(), []Answer{{Raw: "AMZN MKTP EG*4821", NewName: "amazon.eg"}})
	if len(got) != 1 || got[0].MatchID != "cp-amazon" {
		t.Errorf("a new merchant named like an alias = %+v, want a match", got)
	}
}

func TestParseLongName(t *testing.T) {
	got := Parse(testIn(), []Answer{{Raw: "UBER *TRIP", NewName: strings.Repeat("x", MaxNameLen+1)}})
	if len(got) != 0 {
		t.Errorf("a too long name = %+v, want dropped", got)
	}
}

func TestPrompt(t *testing.T) {
	p := Prompt(testIn())
	for _, want := range []string{"- AMZN MKTP EG*4821\n", "M1: Amazon | AMAZON.EG\n", "M2: Carrefour\n", "C2: Transport\n"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	for _, id := range []string{"cp-amazon", "cat-food"} {
		if strings.Contains(p, id) {
			t.Errorf("prompt has the id %q, refs only", id)
		}
	}
}
