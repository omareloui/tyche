package countries

import "time"

// satFirst and sunFirst are the countries whose weeks start on Saturday or
// Sunday (CLDR's firstDay), the others' on Monday.
var (
	satFirst = map[string]bool{
		"AE": true, "AF": true, "BH": true, "DJ": true, "DZ": true, "EG": true,
		"IQ": true, "IR": true, "JO": true, "KW": true, "LY": true, "OM": true,
		"QA": true, "SD": true, "SY": true,
	}
	sunFirst = map[string]bool{
		"AG": true, "AS": true, "BD": true, "BR": true, "BS": true, "BT": true,
		"BW": true, "BZ": true, "CA": true, "CN": true, "CO": true, "DM": true,
		"DO": true, "ET": true, "GT": true, "GU": true, "HK": true, "HN": true,
		"ID": true, "IL": true, "IN": true, "JM": true, "JP": true, "KE": true,
		"KH": true, "KR": true, "LA": true, "MH": true, "MM": true, "MO": true,
		"MT": true, "MX": true, "MZ": true, "NI": true, "NP": true, "PA": true,
		"PE": true, "PH": true, "PK": true, "PR": true, "PT": true, "PY": true,
		"SA": true, "SG": true, "SV": true, "TH": true, "TT": true, "TW": true,
		"UM": true, "US": true, "VE": true, "VI": true, "WS": true, "YE": true,
		"ZA": true, "ZW": true,
	}
)

// WeekStart is the day weeks start on in the country of a 2 letter code,
// Monday when unknown.
func WeekStart(iso2 string) time.Weekday {
	switch {
	case satFirst[iso2]:
		return time.Saturday
	case sunFirst[iso2]:
		return time.Sunday
	}
	return time.Monday
}

// WeekStartOfTimezone is the day weeks start on in an IANA time zone's
// country, Monday when unknown.
func WeekStartOfTimezone(tz string) time.Weekday {
	if c, ok := FromTimezone(tz); ok {
		return WeekStart(c.ISO2)
	}
	return time.Monday
}
