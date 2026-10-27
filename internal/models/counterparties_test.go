package models

import (
	"slices"
	"testing"
)

func TestCounterpartyAbsorb(t *testing.T) {
	cat := &Category{ID: "cat"}

	tests := []struct {
		name        string
		into        Counterparty
		other       Counterparty
		wantAliases []string
		wantCat     *Category
	}{
		{
			name:        "adds name and aliases",
			into:        Counterparty{Name: "Uber", Aliases: []string{"UBER BV"}},
			other:       Counterparty{Name: "Uber Trip", Aliases: []string{"UBER *TRIP"}},
			wantAliases: []string{"UBER BV", "Uber Trip", "UBER *TRIP"},
		},
		{
			name:        "skips case-insensitive duplicates, own name and blanks",
			into:        Counterparty{Name: "Uber", Aliases: []string{"UBER BV"}},
			other:       Counterparty{Name: "uber bv", Aliases: []string{"UBER", " ", "Uber Bv"}},
			wantAliases: []string{"UBER BV"},
		},
		{
			name:        "takes default category for merchant without one",
			into:        Counterparty{Name: "Uber", IsMerchant: true},
			other:       Counterparty{Name: "Uber Trip", DefaultCategory: cat},
			wantAliases: []string{"Uber Trip"},
			wantCat:     cat,
		},
		{
			name:        "keeps own default category",
			into:        Counterparty{Name: "Uber", IsMerchant: true, DefaultCategory: &Category{ID: "own"}},
			other:       Counterparty{Name: "Uber Trip", DefaultCategory: cat},
			wantAliases: []string{"Uber Trip"},
			wantCat:     &Category{ID: "own"},
		},
		{
			name:        "non merchant doesn't take default category",
			into:        Counterparty{Name: "Ahmed"},
			other:       Counterparty{Name: "Ahmed M", DefaultCategory: cat},
			wantAliases: []string{"Ahmed M"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.into.Absorb(&tt.other)
			if !slices.Equal(tt.into.Aliases, tt.wantAliases) {
				t.Errorf("aliases = %q, want %q", tt.into.Aliases, tt.wantAliases)
			}
			gotCat, wantCat := "", ""
			if tt.into.DefaultCategory != nil {
				gotCat = tt.into.DefaultCategory.ID
			}
			if tt.wantCat != nil {
				wantCat = tt.wantCat.ID
			}
			if gotCat != wantCat {
				t.Errorf("default category = %q, want %q", gotCat, wantCat)
			}
		})
	}
}
