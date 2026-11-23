package payroll

import (
	"slices"
	"sort"
	"strings"
)

// builtin is a country's rules written here, for its latest known year.
type builtin struct {
	options []Option
	// rules are the year's rules for the picked options (each option's
	// value one of its choices, the first when not picked).
	rules func(opts map[string]string) Rules
}

var builtins = map[string]builtin{
	"EG": egypt,
	"ES": spain,
	"SA": saudi,
	"AE": uae,
	"CA": canada,
	"GB": uk,
	"US": us,
}

// BuiltinCountries are the ISO2 codes of the countries with rules here,
// sorted.
func BuiltinCountries() []string {
	out := make([]string, 0, len(builtins))
	for c := range builtins {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// HasBuiltin tells the country has rules here.
func HasBuiltin(country string) bool {
	_, ok := builtins[strings.ToUpper(country)]
	return ok
}

// BuiltinOptions are the choices the country's rules depend on.
func BuiltinOptions(country string) []Option {
	return builtins[strings.ToUpper(country)].options
}

// PickOptions keeps the values of opts that are one of the options'
// choices, the first choice for the others.
func PickOptions(options []Option, opts map[string]string) map[string]string {
	out := make(map[string]string, len(options))
	for _, o := range options {
		v := opts[o.Key]
		if !slices.ContainsFunc(o.Choices, func(c Choice) bool { return c.Value == v }) {
			v = o.Choices[0].Value
		}
		out[o.Key] = v
	}
	return out
}

// Variant names the picked options, e.g. "filing=single".
func Variant(options []Option, opts map[string]string) string {
	var parts []string
	for _, o := range options {
		parts = append(parts, o.Key+"="+opts[o.Key])
	}
	return strings.Join(parts, ",")
}

// Builtin is the country's rules here for the options (PickOptions), false
// when there are none. Their Year is the one they were written for, which
// may be before the one asked.
func Builtin(country string, opts map[string]string) (*Rules, bool) {
	country = strings.ToUpper(country)
	b, ok := builtins[country]
	if !ok {
		return nil, false
	}
	picked := PickOptions(b.options, opts)
	r := b.rules(picked)
	r.Country, r.Source, r.Variant = country, SourceBuiltin, Variant(b.options, picked)
	return &r, true
}
