package sanitizer

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/leebenson/conform"
)

type Sanitizer struct{}

func New() *Sanitizer {
	conform.AddSanitizer("nospaces", func(in string) string {
		re := regexp.MustCompile(`\s`)
		return re.ReplaceAllString(in, "")
	})
	conform.AddSanitizer("titlecase", Title)
	return new(Sanitizer)
}

func (s *Sanitizer) SanitizeStruct(in interface{}) error {
	return conform.Strings(in)
}

func (s *Sanitizer) Trim(str string) string {
	return strings.Trim(strings.TrimSpace(str), "\x1c\x1d\x1e\x1f")
}

func (s *Sanitizer) TrimMap(m map[string]string) map[string]string {
	nm := map[string]string{}
	for k, v := range m {
		nm[s.Trim(k)] = s.Trim(v)
	}
	return nm
}

func (s *Sanitizer) Lower(str string) string {
	return strings.ToLower(str)
}

func (s *Sanitizer) Upper(str string) string {
	return strings.ToUpper(str)
}

// Title uppercases the first letter of every word, like strings.Title but
// without treating apostrophes as word breaks ("chili's" → "Chili's", not "Chili'S").
func Title(s string) string {
	prev := ' '
	return strings.Map(func(r rune) rune {
		inWord := unicode.IsLetter(prev) || unicode.IsDigit(prev) || unicode.IsMark(prev) || prev == '\'' || prev == '’'
		prev = r
		if inWord {
			return r
		}
		return unicode.ToTitle(r)
	}, s)
}
