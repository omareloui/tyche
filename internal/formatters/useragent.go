package formatters

import "strings"

// UserAgent names the browser and OS of a User-Agent header, like
// "Firefox on Linux"; "" when neither is known.
func UserAgent(ua string) string {
	browser := firstMatch(ua, [][2]string{
		{"Edg", "Edge"},
		{"OPR/", "Opera"},
		{"SamsungBrowser/", "Samsung Internet"},
		{"FxiOS/", "Firefox"},
		{"Firefox/", "Firefox"},
		{"CriOS/", "Chrome"},
		{"Chrome/", "Chrome"},
		{"Safari/", "Safari"},
	})
	os := firstMatch(ua, [][2]string{
		{"Windows", "Windows"},
		{"Android", "Android"},
		{"iPhone", "iOS"},
		{"iPad", "iPadOS"},
		{"Mac OS X", "macOS"},
		{"CrOS", "ChromeOS"},
		{"Linux", "Linux"},
	})
	switch {
	case browser != "" && os != "":
		return browser + " on " + os
	case browser != "":
		return browser
	default:
		return os
	}
}

func firstMatch(s string, needles [][2]string) string {
	for _, n := range needles {
		if strings.Contains(s, n[0]) {
			return n[1]
		}
	}
	return ""
}
