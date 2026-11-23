// Package taxguess has Gemini look up a country's (or region's) income tax
// and employee social contributions (grounded on Google Search), as
// payroll.Rules, for the salary calculator's countries without built in
// rules. Only an estimate: the page says it may be wrong.
package taxguess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/colguess"
	"github.com/omareloui/tyche/internal/gemini"
	"github.com/omareloui/tyche/internal/payroll"
)

// ErrNoRules is an answer that can't be used.
var ErrNoRules = errors.New("couldn't find the tax rules")

// In is what to look up.
type In struct {
	// Country is its ISO 3166 alpha-2 code, Name its name, Currency its
	// (ISO 4217).
	Country, Name, Currency string
	// Region is an optional state / province / region, as typed.
	Region string
	Year   int
}

// keyVersion starts the cache keys, bumped when the prompt changes what the
// answers mean.
const keyVersion = "v1|"

// Key is the cache key of the rules: country, region (normalized) and year.
func (in In) Key() string {
	return Key(in.Country, in.Region, in.Year)
}

// Key is the cache key of a country's (region's) rules of a year.
func Key(country, region string, year int) string {
	return keyVersion + strings.ToUpper(strings.TrimSpace(country)) + "|" + colguess.NormalizeCity(region) + "|" + strconv.Itoa(year)
}

// Year is the tax year asked for now.
func Year(now time.Time) int { return now.UTC().Year() }

// Guesser looks the rules up (an interface so tests fake it).
type Guesser interface {
	Guess(ctx context.Context, in In) (*payroll.Rules, gemini.Usage, error)
	Model() string
}

// Client looks up with Gemini.
type Client struct {
	*gemini.Client
}

// New is a client of the model, nil without an API key.
func New(apiKey, model, thinkingLevel string) *Client {
	c := gemini.New(apiKey, model, thinkingLevel)
	if c == nil {
		return nil
	}
	return &Client{Client: c}
}

var bracketSchema = map[string]any{
	"type": "array",
	"items": map[string]any{
		"type": "object",
		"properties": map[string]any{
			"up_to": map[string]any{"type": "number", "description": "upper limit of the band, a year's; 0 for the last, open ended band"},
			"rate":  map[string]any{"type": "number", "description": "fraction, e.g. 0.2 for 20%"},
		},
		"required": []string{"up_to", "rate"},
	},
}

var responseSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"found":    map[string]any{"type": "boolean", "description": "false when you can't tell the rules"},
		"year":     map[string]any{"type": "integer", "description": "the tax year these rules are of"},
		"currency": map[string]any{"type": "string", "description": "ISO 4217 code"},
		"region":   map[string]any{"type": "string", "description": "the region the rules are of, its usual English name; empty for the whole country"},
		"brackets": bracketSchema,
		"tables": map[string]any{
			"type":        "array",
			"description": "only when higher incomes are taxed on another scale altogether; usually empty",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"above":    map[string]any{"type": "number"},
					"brackets": bracketSchema,
				},
				"required": []string{"above", "brackets"},
			},
		},
		"allowance": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"amount":     map[string]any{"type": "number"},
				"taper_from": map[string]any{"type": "number"},
				"taper_rate": map[string]any{"type": "number"},
				"as_credit":  map[string]any{"type": "boolean"},
			},
			"required": []string{"amount", "taper_from", "taper_rate", "as_credit"},
		},
		"deductions": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":   map[string]any{"type": "string"},
					"amount": map[string]any{"type": "number"},
				},
				"required": []string{"name", "amount"},
			},
		},
		"contributions": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":       map[string]any{"type": "string"},
					"rate":       map[string]any{"type": "number"},
					"floor":      map[string]any{"type": "number"},
					"cap":        map[string]any{"type": "number"},
					"monthly":    map[string]any{"type": "boolean"},
					"deductible": map[string]any{"type": "boolean"},
					"bands":      bracketSchema,
				},
				"required": []string{"name", "rate", "floor", "cap", "monthly", "deductible"},
			},
		},
		"pays_per_year": map[string]any{"type": "integer"},
		"notes":         map[string]any{"type": "string"},
	},
	"required": []string{"found", "year", "currency", "region", "brackets", "allowance", "deductions", "contributions", "pays_per_year", "notes"},
}

// Prompt asks for the rules of an employee's salary.
func Prompt(in In) string {
	where := in.Name
	if r := strings.TrimSpace(in.Region); r != "" {
		where = r + ", " + in.Name
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Give the rules that turn an employee's gross salary into take-home pay in %s for the tax year %d (the latest known if %d's aren't out yet), from official sources (the tax authority, the social security institution) or reputable tax guides (PwC Worldwide Tax Summaries, KPMG, Deloitte).\n\n", where, in.Year, in.Year)
	fmt.Fprintf(&b, "The employee: a tax resident, single, no children, a regular full-time employee, no other income, standard deductions only. Every amount is a year's (but contribution floors/caps when monthly is true) in the local currency (%s).\n\n", in.Currency)
	b.WriteString("Fields:\n")
	b.WriteString("- brackets: the income tax scale on the taxable income, ascending; up_to is each band's upper limit, 0 for the last one; rate a fraction (0.2 = 20%). Include a 0% band when the scale starts with one. National and regional/local income taxes summed into one scale when they share the base; when there's no income tax, one band [{up_to: 0, rate: 0}].\n")
	b.WriteString("- tables: only when incomes over a threshold are taxed on a whole other scale (\"above\" = the taxable income threshold); else empty.\n")
	b.WriteString("- allowance: the personal allowance / tax-free amount; taper_from/taper_rate when it shrinks with income (else 0); as_credit true when it's given as a tax credit worked out by taxing it at the scale.\n")
	b.WriteString("- deductions: fixed amounts every employee deducts from the taxable income (standard deduction, employment expenses), not the allowance.\n")
	b.WriteString("- contributions: the EMPLOYEE's mandatory social contributions only (pension, health, unemployment, solidarity...), not the employer's: rate a fraction of the gross between floor and cap (0 = none), monthly true when floor/cap are monthly amounts; deductible true when they come off the taxable income; bands instead of rate when the rate depends on the pay (then rate 0).\n")
	b.WriteString("- pays_per_year: how many salary payments a year is customary (12, or 13/14 where extra months are usual).\n")
	b.WriteString("- notes: one or two short sentences on what's simplified or left out (local taxes, church tax, tax credits).\n")
	b.WriteString("- found: false when you can't tell the rules of this place.\n\n")
	b.WriteString("Rates are fractions between 0 and 1. Answer with the JSON only.")
	return b.String()
}

// Guess asks the model, grounded on Google Search; when the model refuses
// the search with a JSON answer, it asks again without it.
func (c *Client) Guess(ctx context.Context, in In) (*payroll.Rules, gemini.Usage, error) {
	parts := []gemini.Part{{Text: Prompt(in)}}
	ans, usage, err := c.GenerateWith(ctx, parts, responseSchema, gemini.Options{Search: true})
	if err != nil && !usage.Billed() && ctx.Err() == nil {
		var again gemini.Usage
		ans, again, err = c.Generate(ctx, parts, responseSchema)
		usage = usage.Add(again)
	}
	if err != nil {
		return nil, usage, err
	}
	r, err := Parse(ans.Text, in)
	if err != nil {
		return nil, usage, fmt.Errorf("%w (finish reason %s)", err, ans.FinishReason)
	}
	for _, s := range ans.Sources {
		if s.URL != "" && len(r.Sources) < 10 {
			r.Sources = append(r.Sources, s.URL)
		}
	}
	return r, usage, nil
}

type answer struct {
	payroll.Rules
	Found bool `json:"found"`
}

// percents turns a rate given in percent (20 for 20%) into a fraction.
func percents(r float64) float64 {
	if r > 1 && r <= 100 {
		return r / 100
	}
	return r
}

func fixBrackets(bs []payroll.Bracket) {
	for i := range bs {
		bs[i].Rate = percents(bs[i].Rate)
	}
}

// Parse reads and checks an answer as the rules of in.
func Parse(text string, in In) (*payroll.Rules, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimSuffix(strings.TrimPrefix(text, "```"), "```")
	var a answer
	if err := json.Unmarshal([]byte(text), &a); err != nil {
		return nil, fmt.Errorf("%w: unreadable answer", ErrNoRules)
	}
	if !a.Found {
		return nil, fmt.Errorf("%w: not found", ErrNoRules)
	}
	r := a.Rules
	fixBrackets(r.Brackets)
	for i := range r.Tables {
		fixBrackets(r.Tables[i].Brackets)
	}
	for i := range r.Contributions {
		c := &r.Contributions[i]
		c.Rate = percents(c.Rate)
		c.Name = strings.TrimSpace(c.Name)
		fixBrackets(c.Bands)
	}
	r.Allowance.TaperRate = percents(r.Allowance.TaperRate)
	r.Currency = strings.ToUpper(strings.TrimSpace(r.Currency))
	if r.Currency == "" {
		r.Currency = in.Currency
	}
	if r.Year <= 0 {
		r.Year = in.Year
	}
	r.Country = strings.ToUpper(in.Country)
	r.Region = strings.TrimSpace(r.Region)
	if r.Region == "" {
		r.Region = strings.TrimSpace(in.Region)
	}
	r.Notes = strings.TrimSpace(r.Notes)
	r.Source = payroll.SourceAI
	r.WorkReduction = nil
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoRules, err)
	}
	return &r, nil
}
