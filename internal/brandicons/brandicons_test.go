package brandicons_test

import (
	"testing"

	"github.com/omareloui/tyche/internal/brandicons"
)

func TestTitleToSlug(t *testing.T) {
	cases := map[string]string{
		"Booking.com":      "bookingdotcom",
		"C++":              "cplusplus",
		"Société Générale": "societegenerale",
		"AT&T":             "atandt",
		"Łódź":             "lodz",
		"/e/":              "e",
	}
	for in, want := range cases {
		if got := brandicons.TitleToSlug(in); got != want {
			t.Errorf("TitleToSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadSkipsMissingSvgs(t *testing.T) {
	idx, err := brandicons.Load(brandicons.SimpleIcons("testdata/array"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx.Get("missingicon"); ok {
		t.Error("expected icon without svg to be skipped")
	}
	icon, ok := idx.Get("netflix")
	if !ok || icon.Hex != "E50914" || icon.Title != "Netflix" {
		t.Errorf("unexpected netflix icon %+v (ok: %v)", icon, ok)
	}
}

func TestLoadLegacyFormat(t *testing.T) {
	idx, err := brandicons.Load(brandicons.SimpleIcons("testdata/legacy"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx.Get("uber"); !ok {
		t.Error("expected uber in legacy index")
	}
}

func TestDetect(t *testing.T) {
	idx, err := brandicons.Load(brandicons.SimpleIcons("testdata/array"))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		aliases []string
		want    string
	}{
		{"Netflix", nil, "netflix"},
		{"netflix inc", nil, "netflix"},
		{"Booking.com", nil, "bookingdotcom"},
		{"Societe Generale", nil, "societegenerale"},
		{"SocGen", nil, "societegenerale"},
		{"Vodafone Cash", nil, "vodafone"},
		{"My Ride", []string{"UBER *TRIP", "UBER BV"}, "uber"},
		{"VODAFONE EG", nil, "vodafone"},
		{"Ahmed", []string{"E", "E BANK"}, ""},
		{"Unknown Shop", nil, ""},
	}
	for _, c := range cases {
		if got := idx.Detect(c.name, c.aliases); got != c.want {
			t.Errorf("Detect(%q, %v) = %q, want %q", c.name, c.aliases, got, c.want)
		}
	}
}

func TestIsDark(t *testing.T) {
	cases := map[string]bool{
		"000000":  true,
		"#000000": true,
		"E50914":  false,
		"FFFFFF":  false,
		"zzz":     false,
	}
	for in, want := range cases {
		if got := brandicons.IsDark(in); got != want {
			t.Errorf("IsDark(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestIsLight(t *testing.T) {
	cases := map[string]bool{
		"FFFFFF": true,
		"FFDD00": true,
		"2563EB": false,
		"000000": false,
		"zzz":    false,
	}
	for in, want := range cases {
		if got := brandicons.IsLight(in); got != want {
			t.Errorf("IsLight(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestColorFor(t *testing.T) {
	idx, err := brandicons.Load(brandicons.SimpleIcons("testdata/array"))
	if err != nil {
		t.Fatal(err)
	}
	if got := idx.ColorFor("netflix", "2563EB"); got != "E50914" {
		t.Errorf("ColorFor known icon = %q, want brand hex", got)
	}
	if got := idx.ColorFor("", "2563EB"); got != "2563EB" {
		t.Errorf("ColorFor no icon = %q, want current color kept", got)
	}
	if got := idx.ColorFor("", ""); len(got) != 6 || brandicons.IsLight(got) {
		t.Errorf("ColorFor no icon nor color = %q, want a dark enough random color", got)
	}
}

func TestLoadExtraTakesPrecedence(t *testing.T) {
	idx, err := brandicons.Load(brandicons.Extra("testdata/extra"), brandicons.SimpleIcons("testdata/array"))
	if err != nil {
		t.Fatal(err)
	}
	if icon, _ := idx.Get("acme"); icon.Hex != "123456" {
		t.Errorf("acme hex = %q, want extra source's 123456", icon.Hex)
	}
	if icon, _ := idx.Get("netflix"); icon.Hex != "E50914" {
		t.Errorf("netflix hex = %q, want simple-icons' E50914", icon.Hex)
	}

	cases := map[string]string{
		"Amazon":          "amazon",
		"AMAZON Prime EG": "amazon",
		"Amazon Marketpl": "amazon",
		"UBER *TRIP":      "uber",
		"Netflix":         "netflix",
	}
	for name, want := range cases {
		if got := idx.Detect(name, nil); got != want {
			t.Errorf("Detect(%q) = %q, want %q", name, got, want)
		}
	}
}
