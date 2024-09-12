package sanitizer

import (
	"strings"

	"github.com/leebenson/conform"
)

type Sanitizer struct{}

func New() *Sanitizer {
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
