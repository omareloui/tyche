package handler

import (
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/models"
)

func TestParseCounterpartyLocations(t *testing.T) {
	form := url.Values{
		"location_id":       {"01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZD", "", ""},
		"location_label":    {" Zamalek ", "", "Maadi"},
		"location_address":  {"26 July St", " ", ""},
		"location_maps_url": {"", "", "https://maps.app.goo.gl/x"},
	}
	r := httptest.NewRequest("PUT", "/counterparties/x", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	got := parseCounterpartyLocations(r)
	want := []models.CounterpartyLocation{
		{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZD", Label: "Zamalek", Address: "26 July St"},
		{Label: "Maadi", MapsURL: "https://maps.app.goo.gl/x"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("locations = %+v, want %+v", got, want)
	}
}

func TestParseRating(t *testing.T) {
	for in, want := range map[string]int{"": 0, "3": 3, " 5 ": 5, "6": 0, "-1": 0, "x": 0} {
		if got := parseRating(in); got != want {
			t.Errorf("parseRating(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestClearNonMerchantFieldsKeepsNote(t *testing.T) {
	cp := &models.Counterparty{
		Name: "Ahmed", Note: "Cousin", Rating: 4, Icon: "uber",
		Locations: []models.CounterpartyLocation{{Label: "Home"}},
	}
	clearNonMerchantFields(cp)
	if cp.Rating != 0 || cp.Locations != nil || cp.Icon != "" {
		t.Errorf("merchant fields kept: %+v", cp)
	}
	if cp.Note != "Cousin" {
		t.Errorf("note = %q, want Cousin", cp.Note)
	}
}
