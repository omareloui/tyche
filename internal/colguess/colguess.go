// Package colguess has Gemini estimate how a city's prices compare with
// another's (grounded on Google Search), for the cost of living calculator's
// city mode. Only an estimate: the page says it may be unreliable.
package colguess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/omareloui/tyche/internal/gemini"
	"github.com/omareloui/tyche/internal/models"
	"golang.org/x/text/unicode/norm"
)

// ErrNoEstimate is an answer that can't be used.
var ErrNoEstimate = errors.New("couldn't estimate the prices")

// City is a city of a country.
type City struct {
	Name string
	// Country is the country's name, Code its ISO 3166 alpha-2 code.
	Country string
	Code    string
	// Currency is the country's (ISO 4217).
	Currency string
}

// In is what to estimate: target's prices against home's.
type In struct {
	Home, Target City
}

// Key is the cache key of an estimate: both cities (NormalizeCity) with
// their country codes, and the month.
func (in In) Key(now time.Time) string {
	return Key(in.Home.Name, in.Home.Code, in.Target.Name, in.Target.Code, now)
}

// Key is the cache key of an estimate of target city's prices against
// home's in now's month.
func Key(homeCity, homeCode, targetCity, targetCode string, now time.Time) string {
	part := func(city, code string) string {
		return NormalizeCity(city) + "," + strings.ToUpper(strings.TrimSpace(code))
	}
	return part(homeCity, homeCode) + "|" + part(targetCity, targetCode) + "|" + now.UTC().Format("2006-01")
}

// NormalizeCity folds the ways a city's name is typed: lowercased, accents
// dropped (Düsseldorf = dusseldorf), punctuation as spaces (St. Louis = st
// louis), whitespace folded. Letters of other scripts are kept.
func NormalizeCity(name string) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(name) {
		switch {
		case unicode.Is(unicode.Mn, r):
			// An accent, split off its letter by NFKD.
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Estimator estimates (an interface so tests fake it).
type Estimator interface {
	Estimate(ctx context.Context, in In) (*models.ColCityEstimate, gemini.Usage, error)
	Model() string
}

// Client estimates with Gemini.
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

var responseSchema = func() map[string]any {
	groups := map[string]any{}
	var required []string
	for _, g := range models.ColGroups {
		groups[string(g.Group)] = map[string]any{"type": "number", "description": g.Label}
		required = append(required, string(g.Group))
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"overall":     map[string]any{"type": "number"},
			"groups":      map[string]any{"type": "object", "properties": groups, "required": required},
			"home_city":   map[string]any{"type": "string", "description": "the first city's usual English name"},
			"target_city": map[string]any{"type": "string", "description": "the second city's usual English name"},
			"rent_home":   map[string]any{"type": "number"},
			"rent_target": map[string]any{"type": "number"},
			"notes":       map[string]any{"type": "string"},
		},
		"required": []string{"home_city", "target_city", "overall", "groups", "rent_home", "rent_target", "notes"},
	}
}()

// Prompt asks for the price levels of target against home.
func Prompt(in In) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Compare the cost of living in %s (%s) against %s (%s), as of today, using recent data (e.g. Numbeo, Expatistan, national statistics, rental listings).\n\n",
		in.Target.Name, in.Target.Country, in.Home.Name, in.Home.Country)
	b.WriteString("Give price levels of the second city's against the first's, both converted to the same currency at today's market exchange rate: 1 = as expensive, 1.5 = 50% more expensive, 0.6 = 40% cheaper.\n\n")
	b.WriteString("- home_city, target_city: each city's usual English name, as written in English (e.g. \"New York City\" for NYC, \"Cairo\" for القاهرة); empty when you can't tell which city it is\n")
	b.WriteString("- overall: the whole of a typical household's spending, rent included\n")
	b.WriteString("- groups: each group of spending:\n")
	for _, g := range models.ColGroups {
		fmt.Fprintf(&b, "  - %s: %s\n", g.Group, g.Label)
	}
	fmt.Fprintf(&b, "- rent_home: a one bedroom flat's monthly rent in the center of %s, in %s\n", in.Home.Name, in.Home.Currency)
	fmt.Fprintf(&b, "- rent_target: the same in %s, in %s\n", in.Target.Name, in.Target.Currency)
	b.WriteString("- notes: one or two short sentences on what drives the difference\n\n")
	b.WriteString("Every level must be a positive number. Answer with the JSON only.")
	return b.String()
}

// Estimate asks the model, grounded on Google Search; when the model refuses
// the search with a JSON answer, it asks again without it.
func (c *Client) Estimate(ctx context.Context, in In) (*models.ColCityEstimate, gemini.Usage, error) {
	parts := []gemini.Part{{Text: Prompt(in)}}
	ans, usage, err := c.GenerateWith(ctx, parts, responseSchema, gemini.Options{Search: true})
	if err != nil && usage.InputTokens == 0 && usage.OutputTokens == 0 && ctx.Err() == nil {
		ans, usage, err = c.Generate(ctx, parts, responseSchema)
	}
	if err != nil {
		return nil, usage, err
	}
	e, err := Parse(ans.Text)
	if err != nil {
		return nil, usage, fmt.Errorf("%w (finish reason %s)", err, ans.FinishReason)
	}
	for _, s := range ans.Sources {
		e.Sources = append(e.Sources, models.ColSource{Title: s.Title, URL: s.URL})
	}
	e.Model = c.Model()
	return e, usage, nil
}

// Parse reads and checks an answer: levels within reason (0.05–20), the
// groups out of range dropped (taking the overall level).
func Parse(text string) (*models.ColCityEstimate, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimSuffix(strings.TrimPrefix(text, "```"), "```")
	var e models.ColCityEstimate
	if err := json.Unmarshal([]byte(text), &e); err != nil {
		return nil, fmt.Errorf("%w: unreadable answer", ErrNoEstimate)
	}
	if !sane(e.Overall) {
		return nil, fmt.Errorf("%w: overall level %v", ErrNoEstimate, e.Overall)
	}
	groups := map[models.ColGroup]float64{}
	for _, g := range models.ColGroups {
		if v := e.Groups[g.Group]; sane(v) {
			groups[g.Group] = v
		}
	}
	e.Groups = groups
	if e.RentHome < 0 || e.RentTarget < 0 {
		e.RentHome, e.RentTarget = 0, 0
	}
	e.Notes = strings.TrimSpace(e.Notes)
	e.HomeCity = cityName(e.HomeCity)
	e.TargetCity = cityName(e.TargetCity)
	return &e, nil
}

// cityName is a name the model gave, trimmed to a sane length.
func cityName(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 80 {
		s = string(r[:80])
	}
	return s
}

func sane(v float64) bool {
	return v >= 0.05 && v <= 20
}
