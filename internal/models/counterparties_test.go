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

func TestCounterpartyAbsorbRatingAndNote(t *testing.T) {
	tests := []struct {
		name       string
		into       Counterparty
		other      Counterparty
		wantRating int
		wantNote   string
	}{
		{
			name:       "merchant takes rating and note",
			into:       Counterparty{Name: "Uber", IsMerchant: true},
			other:      Counterparty{Name: "Uber Trip", Rating: 4, Note: "Late at night"},
			wantRating: 4,
			wantNote:   "Late at night",
		},
		{
			name:       "keeps own rating, joins notes",
			into:       Counterparty{Name: "Uber", IsMerchant: true, Rating: 2, Note: "Pricey"},
			other:      Counterparty{Name: "Uber Trip", Rating: 5, Note: "Fast"},
			wantRating: 2,
			wantNote:   "Pricey\n\nFast",
		},
		{
			name:     "same note once",
			into:     Counterparty{Name: "Uber", Note: "Fast"},
			other:    Counterparty{Name: "Uber Trip", Note: "Fast"},
			wantNote: "Fast",
		},
		{
			name:     "non merchant doesn't take rating",
			into:     Counterparty{Name: "Ahmed"},
			other:    Counterparty{Name: "Ahmed M", Rating: 3, Note: "Cousin"},
			wantNote: "Cousin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.into.Absorb(&tt.other)
			if tt.into.Rating != tt.wantRating {
				t.Errorf("rating = %d, want %d", tt.into.Rating, tt.wantRating)
			}
			if tt.into.Note != tt.wantNote {
				t.Errorf("note = %q, want %q", tt.into.Note, tt.wantNote)
			}
		})
	}
}

func TestCounterpartyLocationMapsLink(t *testing.T) {
	tests := []struct {
		name string
		loc  CounterpartyLocation
		want string
	}{
		{"maps url", CounterpartyLocation{Label: "Zamalek", Address: "26 July St", MapsURL: "https://maps.app.goo.gl/x"}, "https://maps.app.goo.gl/x"},
		{"address search", CounterpartyLocation{Label: "Zamalek", Address: "26 July St, Cairo"}, "https://www.google.com/maps/search/?api=1&query=26+July+St%2C+Cairo"},
		{"label search", CounterpartyLocation{Label: "Zamalek"}, "https://www.google.com/maps/search/?api=1&query=Zamalek"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.loc.MapsLink(); got != tt.want {
				t.Errorf("MapsLink() = %q, want %q", got, tt.want)
			}
		})
	}
}
