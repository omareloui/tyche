package sanitizer

import "testing"

func TestTitle(t *testing.T) {
	cases := map[string]string{
		"chili's":         "Chili's",
		"mcdonald’s cafe": "Mcdonald’s Cafe",
		"coca-cola co":    "Coca-Cola Co",
		"o'reilly books":  "O'reilly Books",
		"vodafone":        "Vodafone",
		"KFC":             "KFC",
		"":                "",
	}
	for in, want := range cases {
		if got := Title(in); got != want {
			t.Errorf("Title(%q) = %q, want %q", in, got, want)
		}
	}
}
