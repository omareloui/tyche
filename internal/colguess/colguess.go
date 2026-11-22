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
	// Ref is the countries' price levels, a starting point for the cities'
	// (nil without them).
	Ref *Reference
}

// Reference is the target country's price levels against the home
// country's at the market rate (the countries mode's), from the World
// Bank's PPPs of Year.
type Reference struct {
	Year    int
	Overall float64
	Groups  map[models.ColGroup]float64
}

// NewReference is the reference of the countries' ratios (target's currency
// per home's) at the market rate, nil without them.
func NewReference(r *models.ColRatios, market float64, year int) *Reference {
	if r == nil || r.Avg <= 0 || market <= 0 {
		return nil
	}
	ref := &Reference{Year: year, Overall: r.Avg / market, Groups: map[models.ColGroup]float64{}}
	for g, v := range r.Groups {
		if v > 0 {
			ref.Groups[g] = v / market
		}
	}
	return ref
}

// keyVersion starts the cache keys, bumped when the prompt changes what the
// answers mean (v2: the old prompt's levels could be home's against target's;
// v3: the countries' levels given as a starting point).
const keyVersion = "v3|"

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
	return keyVersion + part(homeCity, homeCode) + "|" + part(targetCity, targetCode) + "|" + now.UTC().Format("2006-01")
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
			"overall":     map[string]any{"type": "number", "description": "the target city's prices over the home city's"},
			"groups":      map[string]any{"type": "object", "properties": groups, "required": required},
			"home_city":   map[string]any{"type": "string", "description": "the home city's usual English name"},
			"target_city": map[string]any{"type": "string", "description": "the target city's usual English name"},
			"rent_home":   map[string]any{"type": "number"},
			"rent_target": map[string]any{"type": "number"},
			"notes":       map[string]any{"type": "string"},
		},
		"required": []string{"home_city", "target_city", "overall", "groups", "rent_home", "rent_target", "notes"},
	}
}()

// Prompt asks for the price levels of target against home. The cities are
// named by their role everywhere (never "first"/"second": the model swapped
// them, answering home's prices against target's).
func Prompt(in In) string {
	home := fmt.Sprintf("%s (%s)", in.Home.Name, in.Home.Country)
	target := fmt.Sprintf("%s (%s)", in.Target.Name, in.Target.Country)
	var b strings.Builder
	fmt.Fprintf(&b, "Someone living in %s is thinking of moving to %s. Estimate how expensive %s is compared with %s, as of today, using recent data (e.g. Numbeo, Expatistan, national statistics, rental listings).\n\n",
		home, target, target, home)
	fmt.Fprintf(&b, "- Home city: %s, prices in %s\n", home, in.Home.Currency)
	fmt.Fprintf(&b, "- Target city: %s, prices in %s\n\n", target, in.Target.Currency)
	fmt.Fprintf(&b, "Every level is the TARGET city's price divided by the HOME city's price for the same things, both converted to the same currency at today's market exchange rate: 1 = %s is as expensive as %s, 1.5 = %s is 50%% more expensive, 0.6 = %s is 40%% cheaper. ",
		in.Target.Name, in.Home.Name, in.Target.Name, in.Target.Name)
	fmt.Fprintf(&b, "So when %s is the more expensive city the levels are above 1, when it's the cheaper one below 1.\n\n", in.Target.Name)
	fmt.Fprintf(&b, "- home_city: the usual English name of %s, the home city (e.g. \"New York City\" for NYC, \"Cairo\" for القاهرة); empty when you can't tell which city it is\n", in.Home.Name)
	fmt.Fprintf(&b, "- target_city: the same for %s, the target city\n", in.Target.Name)
	fmt.Fprintf(&b, "- overall: %s against %s for the whole of a typical household's spending, rent included\n", in.Target.Name, in.Home.Name)
	fmt.Fprintf(&b, "- groups: %s against %s for each group of spending:\n", in.Target.Name, in.Home.Name)
	for _, g := range models.ColGroups {
		fmt.Fprintf(&b, "  - %s: %s\n", g.Group, g.Label)
	}
	fmt.Fprintf(&b, "- rent_home: a one bedroom flat's monthly rent in the center of %s, in %s\n", in.Home.Name, in.Home.Currency)
	fmt.Fprintf(&b, "- rent_target: the same in %s, in %s\n", in.Target.Name, in.Target.Currency)
	b.WriteString("- notes: one or two short sentences on what drives the difference\n\n")
	if r := in.Ref; r != nil {
		fmt.Fprintf(&b, "For reference, the World Bank's price levels (PPPs of %d, brought to today by inflation) put %s against %s, as national averages at today's market rate, at:\n",
			r.Year, in.Target.Country, in.Home.Country)
		fmt.Fprintf(&b, "- overall: %.2f\n", r.Overall)
		for _, g := range models.ColGroups {
			if v := r.Groups[g.Group]; v > 0 {
				fmt.Fprintf(&b, "- %s: %.2f\n", g.Group, v)
			}
		}
		fmt.Fprintf(&b, "Start from these and adjust them for the two cities: a country's average mixes in its towns and countryside, big cities cost more than their country (rent most of all), and the more so in some countries than others. Move away from them as far as the cities' data (rents, prices) shows, not further.\n\n")
	}
	b.WriteString("Before answering, check the direction: a level is the target city's price over the home city's. Every level must be a positive number. Answer with the JSON only.")
	return b.String()
}

// Estimate asks the model, grounded on Google Search; when the model refuses
// the search with a JSON answer, it asks again without it.
func (c *Client) Estimate(ctx context.Context, in In) (*models.ColCityEstimate, gemini.Usage, error) {
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
	e, err := Parse(ans.Text)
	if err != nil {
		return nil, usage, fmt.Errorf("%w (finish reason %s)", err, ans.FinishReason)
	}
	for _, s := range ans.Sources {
		e.Sources = append(e.Sources, models.ColSource{Title: s.Title, URL: s.URL})
	}
	if Swapped(in, e) {
		return nil, usage, fmt.Errorf("%w: home and target cities swapped", ErrNoEstimate)
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

// Swapped tells an answer that took the home city for the target and the
// other way round (its levels would then be upside down).
func Swapped(in In, e *models.ColCityEstimate) bool {
	home, target := NormalizeCity(in.Home.Name), NormalizeCity(in.Target.Name)
	if home == target || e.HomeCity == "" || e.TargetCity == "" {
		return false
	}
	return NormalizeCity(e.HomeCity) == target && NormalizeCity(e.TargetCity) == home
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
