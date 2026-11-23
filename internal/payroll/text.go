package payroll

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// The advanced parts of the rules a user types as text: bands as
// "up_to:rate% ..." ("*" for the open ended last), tables as one line each
// "above > bands", work reduction steps as one line each "from up_to
// amount slope".

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// pct is a rate as a percent, rounded off float noise.
func pct(r float64) string { return num(math.Round(r*1e6) / 1e4) }

func parseNum(s string) (float64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("%q isn't a number", s)
	}
	return v, nil
}

// FormatBands writes bands as text, e.g. "12570:0 50270:8 *:2".
func FormatBands(bs []Bracket) string {
	parts := make([]string, len(bs))
	for i, b := range bs {
		up := "*"
		if b.UpTo != 0 {
			up = num(b.UpTo)
		}
		parts[i] = up + ":" + pct(b.Rate)
	}
	return strings.Join(parts, " ")
}

// ParseBands reads FormatBands' text (rates in percent).
func ParseBands(s string) ([]Bracket, error) {
	var out []Bracket
	for _, f := range strings.Fields(s) {
		up, rate, ok := strings.Cut(f, ":")
		if !ok {
			return nil, fmt.Errorf("%q: write a band as limit:rate", f)
		}
		var b Bracket
		if up != "*" {
			v, err := parseNum(up)
			if err != nil {
				return nil, err
			}
			b.UpTo = v
		}
		r, err := parseNum(strings.TrimSuffix(rate, "%"))
		if err != nil {
			return nil, err
		}
		b.Rate = r / 100
		out = append(out, b)
	}
	return out, nil
}

// FormatTables writes tables one a line, e.g. "600000 > 55000:10 *:27.5".
func FormatTables(ts []Table) string {
	lines := make([]string, len(ts))
	for i, t := range ts {
		lines[i] = num(t.Above) + " > " + FormatBands(t.Brackets)
	}
	return strings.Join(lines, "\n")
}

// ParseTables reads FormatTables' text.
func ParseTables(s string) ([]Table, error) {
	var out []Table
	for line := range strings.Lines(s) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		above, bands, ok := strings.Cut(line, ">")
		if !ok {
			return nil, fmt.Errorf("%q: write a table as income > bands", strings.TrimSpace(line))
		}
		a, err := parseNum(above)
		if err != nil {
			return nil, err
		}
		bs, err := ParseBands(bands)
		if err != nil {
			return nil, err
		}
		out = append(out, Table{Above: a, Brackets: bs})
	}
	return out, nil
}

// FormatSteps writes reduction steps one a line, "from up_to amount slope"
// (up_to 0: no end).
func FormatSteps(ss []Step) string {
	lines := make([]string, len(ss))
	for i, s := range ss {
		lines[i] = strings.Join([]string{num(s.From), num(s.UpTo), num(s.Amount), num(s.Slope)}, " ")
	}
	return strings.Join(lines, "\n")
}

// ParseSteps reads FormatSteps' text (the slope may be left out).
func ParseSteps(s string) ([]Step, error) {
	var out []Step
	for line := range strings.Lines(s) {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if len(f) < 3 || len(f) > 4 {
			return nil, fmt.Errorf("%q: write a step as from up_to amount slope", strings.TrimSpace(line))
		}
		var v [4]float64
		for i, x := range f {
			n, err := parseNum(x)
			if err != nil {
				return nil, err
			}
			v[i] = n
		}
		out = append(out, Step{From: v[0], UpTo: v[1], Amount: v[2], Slope: v[3]})
	}
	return out, nil
}
