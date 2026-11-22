package countries

import "testing"

func TestFromTimezone(t *testing.T) {
	cases := map[string]string{
		"Africa/Cairo":        "EG",
		"Europe/Berlin":       "DE",
		"America/New_York":    "US",
		"Asia/Dubai":          "AE",
		"Asia/Calcutta":       "IN",
		"Asia/Kolkata":        "IN",
		"Europe/London":       "GB",
		"Australia/Melbourne": "AU",
	}
	for tz, want := range cases {
		c, ok := FromTimezone(tz)
		if !ok || c.ISO2 != want {
			t.Errorf("FromTimezone(%q) = %v %v, want %s", tz, c, ok, want)
		}
	}

	for _, tz := range []string{"", "UTC", "Etc/GMT+3", "Nowhere/Town"} {
		if c, ok := FromTimezone(tz); ok {
			t.Errorf("FromTimezone(%q) = %v, want none", tz, c)
		}
	}
}

func TestCodesAreUnique(t *testing.T) {
	seen2, seen3 := map[string]bool{}, map[string]bool{}
	for _, c := range all {
		if len(c.ISO2) != 2 || len(c.ISO3) != 3 || len(c.Currency) != 3 || c.Name == "" {
			t.Errorf("bad country %+v", c)
		}
		if seen2[c.ISO2] || seen3[c.ISO3] {
			t.Errorf("duplicate country %+v", c)
		}
		seen2[c.ISO2], seen3[c.ISO3] = true, true
	}
	if c, ok := ByISO3("egy"); !ok || c.ISO2 != "EG" {
		t.Errorf("ByISO3(egy) = %v %v", c, ok)
	}
}
