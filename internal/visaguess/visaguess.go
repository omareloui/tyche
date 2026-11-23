// Package visaguess has Gemini look up a visa's financial requirements
// (grounded on Google Search), as a visa.Rule, for the visa readiness
// tool's visas without built in rules. Only an estimate: the page says to
// check with the consulate.
package visaguess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/colguess"
	"github.com/omareloui/tyche/internal/gemini"
	"github.com/omareloui/tyche/internal/visa"
)

// ErrNoRules is an answer that can't be used.
var ErrNoRules = errors.New("couldn't find the visa's requirements")

// In is what to look up.
type In struct {
	// Country is its ISO 3166 alpha-2 code, Name its name, Currency its
	// (ISO 4217).
	Country, Name, Currency string
	// Visa is the visa's name, as typed.
	Visa string
	Year int
}

// keyVersion starts the cache keys, bumped when the prompt changes what the
// answers mean.
const keyVersion = "v1|"

// Key is the cache key of the visa's rules.
func (in In) Key() string { return Key(in.Country, in.Visa, in.Year) }

// Key is the cache key of a country's visa (its name normalized) of a year.
func Key(country, visaName string, year int) string {
	return keyVersion + strings.ToUpper(strings.TrimSpace(country)) + "|" + colguess.NormalizeCity(visaName) + "|" + strconv.Itoa(year)
}

// Year is the year asked for now.
func Year(now time.Time) int { return now.UTC().Year() }

// Guesser looks the rules up (an interface so tests fake it).
type Guesser interface {
	Guess(ctx context.Context, in In) (*visa.Rule, gemini.Usage, error)
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

var kindNames = func() []string {
	out := make([]string, len(visa.Kinds))
	for i, k := range visa.Kinds {
		out[i] = string(k)
	}
	return out
}()

var categoryNames = func() []string {
	out := make([]string, len(visa.Categories))
	for i, c := range visa.Categories {
		out[i] = string(c)
	}
	return out
}()

var responseSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"found":     map[string]any{"type": "boolean", "description": "false when there's no such visa or you can't tell its requirements"},
		"visa_name": map[string]any{"type": "string", "description": "the visa's official English name"},
		"category":  map[string]any{"type": "string", "enum": categoryNames},
		"year":      map[string]any{"type": "integer", "description": "the year these amounts are of"},
		"currency":  map[string]any{"type": "string", "description": "ISO 4217 code the amounts are in"},
		"requirements": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":          map[string]any{"type": "string", "enum": kindNames},
					"period":        map[string]any{"type": "string", "enum": []string{"month", "year", "once"}},
					"amount":        map[string]any{"type": "number", "description": "for the main applicant alone; 0 for a check"},
					"per_dependent": map[string]any{"type": "array", "items": map[string]any{"type": "number"}, "description": "amounts added per dependent, the last one repeating; empty when the same"},
					"group":         map[string]any{"type": "string", "description": "same non-empty name on requirements that are alternatives (any one is enough); else empty"},
					"months_proof":  map[string]any{"type": "integer", "description": "for an income: how many of the last months each has to reach it; else 0"},
					"text":          map[string]any{"type": "string", "description": "one short sentence"},
				},
				"required": []string{"kind", "period", "amount", "per_dependent", "group", "months_proof", "text"},
			},
		},
		"notes": map[string]any{"type": "string"},
	},
	"required": []string{"found", "visa_name", "category", "year", "currency", "requirements", "notes"},
}

// Prompt asks for the visa's requirements.
func Prompt(in In) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Give the requirements of %s's \"%s\" (a visa or residence permit) for applications in %d (the latest known if %d's aren't out yet), from official sources first (the immigration authority, the ministry of foreign affairs, the consulates), else reputable immigration guides.\n\n", in.Name, in.Visa, in.Year, in.Year)
	fmt.Fprintf(&b, "Amounts are for the main applicant alone, in the currency the authority states them in (usually %s), worked out to absolute numbers (e.g. 200%% of a minimum wage of 1,000 a month = 2000 a month).\n\n", in.Currency)
	b.WriteString("Requirements, each one of the kinds:\n")
	b.WriteString("- income: a regular income of any source (salary, freelance, pension, rents), period month or year; months_proof when it has to be reached in each of the last N months (the bank statements asked).\n")
	b.WriteString("- salary: a job offer's gross salary in the country, period month or year.\n")
	b.WriteString("- savings: funds held, period once.\n")
	b.WriteString("- blocked: funds put in a blocked account, period once.\n")
	b.WriteString("- investment: money to invest in the country (property, a company, bonds), period once.\n")
	b.WriteString("- check: a condition that isn't money (remote employer abroad, degree, experience, health insurance, clean record, language), amount 0, period once, the condition in text.\n")
	b.WriteString("A money requirement whose amount isn't stated (\"sufficient funds\") is a check with that text, never an amount of 0. per_dependent: what each dependent adds to that amount (first, second, ... the last repeating); empty when nothing. group: the same name on alternatives (an income OR savings). Leave out fees.\n\n")
	b.WriteString("visa_name: its official English name (e.g. \"Digital Nomad Visa\"). notes: one or two short sentences on what varies (consulates, family, exemptions). found: false when the country has no such visa.\n\n")
	b.WriteString("Answer with the JSON only.")
	return b.String()
}

// Guess asks the model, grounded on Google Search; when the model refuses
// the search with a JSON answer, it asks again without it.
func (c *Client) Guess(ctx context.Context, in In) (*visa.Rule, gemini.Usage, error) {
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
	Found        bool               `json:"found"`
	VisaName     string             `json:"visa_name"`
	Category     string             `json:"category"`
	Year         int                `json:"year"`
	Currency     string             `json:"currency"`
	Requirements []visa.Requirement `json:"requirements"`
	Notes        string             `json:"notes"`
}

// Parse reads and checks an answer as the rules of in.
func Parse(text string, in In) (*visa.Rule, error) {
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
	r := visa.Rule{
		Country:  strings.ToUpper(in.Country),
		Name:     strings.TrimSpace(a.VisaName),
		Category: visa.ParseCategory(a.Category),
		Year:     a.Year,
		Currency: strings.ToUpper(strings.TrimSpace(a.Currency)),
		Source:   visa.SourceAI,
		Notes:    strings.TrimSpace(a.Notes),
	}
	if r.Name == "" {
		r.Name = strings.TrimSpace(in.Visa)
	}
	r.Key = visa.KeyOf(r.Name)
	if r.Currency == "" {
		r.Currency = in.Currency
	}
	if r.Year <= 0 {
		r.Year = in.Year
	}
	for _, q := range a.Requirements {
		if q, ok := fixRequirement(q); ok && len(r.Requirements) < visa.MaxRequirements {
			r.Requirements = append(r.Requirements, q)
		}
	}
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoRules, err)
	}
	return &r, nil
}

// fixRequirement repairs what the model gets loosely wrong rather than
// refusing the whole answer: a money requirement without an amount (funds
// "sufficient" for the stay) becomes a condition, a period not fitting its
// kind is put right, odd per dependent amounts and months are dropped;
// false when nothing's left of it (an unknown kind, no text).
func fixRequirement(q visa.Requirement) (visa.Requirement, bool) {
	q.Kind = visa.Kind(strings.ToLower(strings.TrimSpace(string(q.Kind))))
	q.Period = visa.Period(strings.ToLower(strings.TrimSpace(string(q.Period))))
	q.Text = strings.TrimSpace(q.Text)
	q.Group = strings.TrimSpace(q.Group)
	if !slices.Contains(visa.Kinds, q.Kind) {
		return q, false
	}
	if q.Kind.Money() && (q.Amount <= 0 || math.IsNaN(q.Amount) || math.IsInf(q.Amount, 0)) {
		if q.Text == "" {
			return q, false
		}
		q.Kind = visa.KindCheck
	}
	if q.Kind == visa.KindCheck {
		if q.Text == "" {
			return q, false
		}
		q.Amount, q.Period, q.PerDependent, q.MonthsProof, q.Group = 0, "", nil, 0, ""
		return q, true
	}
	switch q.Kind {
	case visa.KindIncome, visa.KindSalary:
		if q.Period != visa.PeriodYear {
			q.Period = visa.PeriodMonth
		}
	default:
		q.Period = visa.PeriodOnce
	}
	deps := q.PerDependent[:0]
	for _, d := range q.PerDependent {
		if d >= 0 && !math.IsNaN(d) && !math.IsInf(d, 0) {
			deps = append(deps, d)
		}
	}
	q.PerDependent = deps
	if allZero(q.PerDependent) {
		q.PerDependent = nil
	}
	if q.Kind != visa.KindIncome || q.MonthsProof < 0 || q.MonthsProof > 24 {
		q.MonthsProof = 0
	}
	return q, true
}

func allZero(vs []float64) bool {
	for _, v := range vs {
		if v != 0 {
			return false
		}
	}
	return true
}
