package smsparser

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	dateRe = `\d{1,2}[/-]\d{1,2}[/-]\d{2,4}|\d{1,2}` + ws + `?[A-Za-z]{3}` + ws + `?\d{2,4}`
	timeRe = `\d{1,2}:\d{2}(?::\d{2})?(?:` + ws + `?[AaPp][Mm])?`
)

// dateLayouts are tried in order to read a date without a format, days come
// first as in Egyptian banks' sms.
var dateLayouts = []string{
	"2/1/06", "2/1/2006", "2-1-06", "2-1-2006",
	"2Jan06", "2Jan2006", "2 Jan 06", "2 Jan 2006",
}

var timeLayouts = []string{
	"15:04", "15:04:05", "3:04 PM", "3:04:05 PM", "3:04PM", "3:04:05PM",
}

// parseDate reads a date with the layout, or the first of dateLayouts that
// works when it's empty.
func parseDate(s, layout string) (time.Time, bool) {
	s = collapseSpaces(s)
	layouts := dateLayouts
	if layout != "" {
		layouts = []string{layout}
	}
	for _, l := range layouts {
		// month names are matched case-insensitively
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseClock returns the time of day as a duration from midnight.
func parseClock(s string) (time.Duration, bool) {
	s = strings.ToUpper(collapseSpaces(s))
	for _, l := range timeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute + time.Duration(t.Second())*time.Second, true
		}
	}
	return 0, false
}

var dateTokens = []struct{ token, layout, re string }{
	{"YYYY", "2006", `\d{4}`},
	{"YY", "06", `\d{2}`},
	{"MMM", "Jan", `[A-Za-z]{3}`},
	{"MM", "1", `\d{1,2}`},
	{"DD", "2", `\d{1,2}`},
}

// dateFormat turns a format like DD/MM/YYYY into a time layout and the
// regexp matching it.
func dateFormat(format string) (layout, re string, err error) {
	var l, r strings.Builder
	hasDay, hasMonth := false, false
	for rest := collapseSpaces(format); rest != ""; {
		matched := false
		for _, t := range dateTokens {
			if strings.HasPrefix(rest, t.token) {
				l.WriteString(t.layout)
				r.WriteString(t.re)
				rest = rest[len(t.token):]
				matched = true
				switch t.token[0] {
				case 'D':
					hasDay = true
				case 'M':
					hasMonth = true
				}
				break
			}
		}
		if matched {
			continue
		}
		c := rest[:1]
		rest = rest[1:]
		if c == " " {
			l.WriteString(" ")
			r.WriteString(ws + `+`)
			continue
		}
		if strings.ContainsAny(c, "0123456789") {
			return "", "", fmt.Errorf("invalid date format %q, use DD, MM, MMM, YY and YYYY", format)
		}
		l.WriteString(c)
		r.WriteString(regexp.QuoteMeta(c))
	}
	if !hasDay || !hasMonth {
		return "", "", fmt.Errorf("date format %q needs a day (DD) and a month (MM or MMM)", format)
	}
	return l.String(), r.String(), nil
}
