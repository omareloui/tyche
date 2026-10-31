package smsparser

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// A template is the text of an sms with placeholders between {{ and }}:
//
//	Your card ending with{{account_id}} was charged for {{currency}} {{amount}} at {{counterparty:merchant}} on {{date}} at {{time}}.
//
// The text has to match the whole sms, any run of whitespace in it matching
// any run of whitespace in the sms. Placeholders ignore the whitespace around
// them, so "with{{account_id}}" also matches "with 1234". They're:
//
//   - {{field}}: a field of the sms, see fields for their names and what they
//     match by default.
//   - {{field:modifier}}: "merchant" for counterparty, a format like
//     DD/MM/YYYY for date.
//   - {{field:/regexp/}}: the field, matching the regexp instead of its
//     default.
//   - {{/regexp/}}: text to skip, matching the regexp, e.g. {{/\d+/}}.
//   - {{*}}: any text to skip, e.g. the end of the sms.

// ws is a whitespace character, \s alone is ASCII only.
const ws = `[\s\p{Z}]`

// Pattern is a compiled template.
type Pattern struct {
	re     *regexp.Regexp
	fields []placeholder
}

type placeholder struct {
	field    string
	modifier string
	// re is what it matches, a regexp.
	re string
	// layout is the date layout from the modifier, empty to detect it.
	layout string
}

type token struct {
	literal string
	ph      *placeholder
}

// CompileError is a mistake in a template.
type CompileError struct {
	// Pos is the byte offset in the template.
	Pos int
	Msg string
}

func (e *CompileError) Error() string {
	return fmt.Sprintf("at %d: %s", e.Pos, e.Msg)
}

// Compile turns a template into a pattern.
func Compile(body string) (*Pattern, error) {
	toks, err := tokenize(body)
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	b.WriteString(`(?s)^` + ws + `*`)
	p := &Pattern{}
	for _, t := range toks {
		if t.ph == nil {
			// the whitespace around it is matched by the placeholders' and the
			// anchors' padding
			b.WriteString(literalRe(strings.TrimSpace(t.literal)))
			continue
		}

		b.WriteString(ws + `*`)
		if t.ph.field == "" {
			b.WriteString(`(?:` + t.ph.re + `)`)
		} else {
			b.WriteString(`(?P<` + t.ph.field + `>` + t.ph.re + `)`)
			p.fields = append(p.fields, *t.ph)
		}
		b.WriteString(ws + `*`)
	}
	b.WriteString(ws + `*$`)

	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, &CompileError{Msg: "invalid pattern: " + err.Error()}
	}
	p.re = re
	return p, nil
}

func tokenize(body string) ([]token, error) {
	var toks []token
	seen := map[string]bool{}

	pos := 0
	for {
		i := strings.Index(body[pos:], "{{")
		if i < 0 {
			if pos < len(body) {
				toks = append(toks, token{literal: body[pos:]})
			}
			break
		}
		if i > 0 {
			toks = append(toks, token{literal: body[pos : pos+i]})
		}

		start := pos + i
		inner := body[start+2:]
		end := closingIndex(inner)
		if end < 0 {
			return nil, &CompileError{Pos: start, Msg: "unclosed {{"}
		}

		ph, err := parsePlaceholder(strings.TrimSpace(inner[:end]))
		if err != nil {
			return nil, &CompileError{Pos: start, Msg: err.Error()}
		}
		if ph.field != "" {
			if seen[ph.field] {
				return nil, &CompileError{Pos: start, Msg: fmt.Sprintf("%q is used more than once", ph.field)}
			}
			seen[ph.field] = true
		}
		toks = append(toks, token{ph: ph})
		pos = start + 2 + end + 2
	}

	if len(toks) == 0 {
		return nil, &CompileError{Msg: "empty template"}
	}
	return toks, nil
}

// closingIndex finds the }} closing a placeholder, the regexps in it can have
// braces so they're skipped up to their closing /}}.
func closingIndex(inner string) int {
	trimmed := strings.TrimLeft(inner, " ")
	isRe := strings.HasPrefix(trimmed, "/")
	if !isRe {
		if c := strings.Index(inner, "}}"); c >= 0 {
			if _, mod, ok := strings.Cut(inner[:c], ":"); ok && strings.HasPrefix(strings.TrimSpace(mod), "/") {
				isRe = true
			}
		}
	}
	if !isRe {
		return strings.Index(inner, "}}")
	}

	open := strings.Index(inner, "/")
	c := strings.Index(inner[open+1:], "/}}")
	if c < 0 {
		return -1
	}
	return open + 1 + c + 1
}

func parsePlaceholder(s string) (*placeholder, error) {
	if s == "*" {
		return &placeholder{re: `.*?`}, nil
	}
	if re, ok := slashed(s); ok {
		if err := checkRe(re); err != nil {
			return nil, err
		}
		return &placeholder{re: re}, nil
	}

	name, mod, _ := strings.Cut(s, ":")
	name, mod = strings.TrimSpace(name), strings.TrimSpace(mod)

	spec, ok := fields[name]
	if !ok {
		return nil, fmt.Errorf("unknown field %q", name)
	}
	ph := &placeholder{field: name, modifier: mod, re: spec.re}

	if re, ok := slashed(mod); ok {
		if err := checkRe(re); err != nil {
			return nil, err
		}
		ph.re, ph.modifier = re, ""
		return ph, nil
	}

	switch {
	case mod == "":
	case name == "counterparty" && mod == "merchant":
	case name == "date":
		layout, re, err := dateFormat(mod)
		if err != nil {
			return nil, err
		}
		ph.layout, ph.re = layout, re
	default:
		return nil, fmt.Errorf("unknown modifier %q for %q", mod, name)
	}
	return ph, nil
}

func slashed(s string) (string, bool) {
	if len(s) >= 2 && s[0] == '/' && s[len(s)-1] == '/' {
		return s[1 : len(s)-1], true
	}
	return "", false
}

func checkRe(re string) error {
	if re == "" {
		return fmt.Errorf("empty regexp")
	}
	if _, err := regexp.Compile(re); err != nil {
		return fmt.Errorf("invalid regexp %q: %v", re, err)
	}
	return nil
}

// literalRe escapes the text, letting its whitespace runs match any
// whitespace.
func literalRe(s string) string {
	var b strings.Builder
	inSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !inSpace {
				b.WriteString(ws + `+`)
			}
			inSpace = true
			continue
		}
		inSpace = false
		b.WriteString(regexp.QuoteMeta(string(r)))
	}
	return b.String()
}

// Fields are the fields of the template in order.
func (p *Pattern) Fields() []string {
	names := make([]string, len(p.fields))
	for i, f := range p.fields {
		names[i] = f.field
	}
	return names
}

// Match returns the raw text of every field, false if the sms doesn't match.
func (p *Pattern) Match(sms string) (map[string]string, bool) {
	m := p.re.FindStringSubmatch(sms)
	if m == nil {
		return nil, false
	}
	vals := make(map[string]string, len(p.fields))
	for i, name := range p.re.SubexpNames() {
		if name != "" {
			vals[name] = collapseSpaces(m[i])
		}
	}
	return vals, true
}

func (p *Pattern) placeholder(field string) *placeholder {
	for i := range p.fields {
		if p.fields[i].field == field {
			return &p.fields[i]
		}
	}
	return nil
}

// collapseSpaces trims the text and turns its whitespace runs into single
// spaces.
func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
