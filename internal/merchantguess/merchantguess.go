// Package merchantguess has Gemini decide what the merchant strings of bank
// sms no counterparty matches are ("AMZN MKTP EG*4821"): one of the user's
// merchants, or a new merchant with a clean name and a default category.
// Only merchant strings, the merchants' names and the expense categories'
// names are sent: never people, amounts or sms bodies.
package merchantguess

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/omareloui/tyche/internal/gemini"
	"github.com/omareloui/tyche/internal/models"
)

// MaxNames is the most merchant strings asked about in a call.
const MaxNames = 50

// MaxNameLen is the longest name of a new merchant, longer ones dropped.
const MaxNameLen = 60

// MaxLabels is the most labels a new merchant gets.
const MaxLabels = 3

// Merchant is one of the user's merchants.
type Merchant struct {
	ID      string
	Name    string
	Aliases []string
}

// Category is an expense category of the user's.
type Category struct {
	ID   string
	Name string
}

// In is what's asked: the merchant strings, with what they can be.
type In struct {
	Names      []string
	Merchants  []Merchant
	Categories []Category
}

// Answer is the model's decision on a merchant string, as answered: a
// match (MatchRef, "M<i>") or a new merchant (NewName, CategoryRef "C<i>").
type Answer struct {
	Raw         string   `json:"raw"`
	MatchRef    string   `json:"match"`
	NewName     string   `json:"new_name"`
	CategoryRef string   `json:"category"`
	Labels      []string `json:"labels"`
}

// Decision is a checked Answer: MatchID one of the merchants' ids, or
// NewName a merchant to create, CategoryID one of the categories' ids or
// empty.
type Decision struct {
	Raw        string
	MatchID    string
	NewName    string
	CategoryID string
	Labels     []string
}

// Guesser decides the strings (an interface so tests fake it).
type Guesser interface {
	Guess(ctx context.Context, in In) ([]Answer, gemini.Usage, error)
	Model() string
}

// Client decides with Gemini.
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

var responseSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"decisions": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"raw":      map[string]any{"type": "string"},
					"match":    map[string]any{"type": "string"},
					"new_name": map[string]any{"type": "string"},
					"category": map[string]any{"type": "string"},
					"labels":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"raw", "match", "new_name", "category"},
			},
		},
	},
	"required": []string{"decisions"},
}

// Guess asks the model, unchecked (see Parse).
func (c *Client) Guess(ctx context.Context, in In) ([]Answer, gemini.Usage, error) {
	ans, usage, err := c.Generate(ctx, []gemini.Part{{Text: Prompt(in)}}, responseSchema)
	if err != nil {
		return nil, usage, err
	}
	var a struct {
		Decisions []Answer `json:"decisions"`
	}
	if err := json.Unmarshal([]byte(ans.Text), &a); err != nil {
		return nil, usage, fmt.Errorf("unreadable answer (finish reason %s)", ans.FinishReason)
	}
	return a.Decisions, usage, nil
}

func merchantRef(i int) string { return "M" + strconv.Itoa(i+1) }
func categoryRef(i int) string { return "C" + strconv.Itoa(i+1) }

// refIndex is the index of a "M<i>" / "C<i>" ref of n items, -1 when it's
// none of them.
func refIndex(ref, prefix string, n int) int {
	ref = strings.ToUpper(strings.TrimSpace(ref))
	if !strings.HasPrefix(ref, prefix) {
		return -1
	}
	i, err := strconv.Atoi(ref[len(prefix):])
	if err != nil || i < 1 || i > n {
		return -1
	}
	return i - 1
}

// Prompt asks to decide every merchant string, the merchants and categories
// referred to by short refs (cheaper than ids, and an invented one is told
// apart).
func Prompt(in In) string {
	var b strings.Builder
	b.WriteString(`You tidy up a personal finance app. Bank SMS name the merchant of a card payment as the card network writes it: abbreviated, upper case, with branch numbers, city names, payment processor prefixes (PAYPRO-*, PAYMOB*, SQ *, PAYPAL *…) and references. None of the user's merchants matches the strings below by name or alias.

For each string decide:
- If it's the same business as one of the user's merchants (another spelling, branch, location or processor prefix of it), answer its ref in "match" (e.g. "M3") and leave new_name and category empty.
- Else answer "match" empty and give the business a clean, short name in "new_name" as people call it (the brand, in title case, e.g. "AMZN MKTP EG*4821" → "Amazon", "CARREFOUR MAADI 0023" → "Carrefour", "UBER *TRIP HELP.UBER.COM" → "Uber"), without branch, city, numbers or processor prefixes. Pick the expense category it most likely belongs to by its ref in "category" (empty when none fits), and up to 3 short "labels" describing the kind of business (e.g. "Supermarket", "Restaurant", "Online store", "Transport", "Pharmacy").
- Only match when sure it's the same business: a different business of the same kind is a new merchant.
Answer one decision per string, with "raw" the string exactly as given.

`)
	b.WriteString("Strings:\n")
	for _, n := range in.Names {
		b.WriteString("- ")
		b.WriteString(n)
		b.WriteByte('\n')
	}
	b.WriteString("\nThe user's merchants (ref: name, then aliases):\n")
	if len(in.Merchants) == 0 {
		b.WriteString("(none)\n")
	}
	for i, m := range in.Merchants {
		fmt.Fprintf(&b, "%s: %s", merchantRef(i), m.Name)
		if len(m.Aliases) > 0 {
			fmt.Fprintf(&b, " | %s", strings.Join(m.Aliases, " | "))
		}
		b.WriteByte('\n')
	}
	b.WriteString("\nExpense categories (ref: name):\n")
	if len(in.Categories) == 0 {
		b.WriteString("(none)\n")
	}
	for i, c := range in.Categories {
		fmt.Fprintf(&b, "%s: %s\n", categoryRef(i), c.Name)
	}
	return b.String()
}

// Parse checks the answers against what was asked: one decision per string
// asked (the first), refs among the ones sent, a new merchant named like
// one of the merchants (any case, name or alias) taken as a match to it.
// Anything else is dropped, leaving that string as it was.
func Parse(in In, answers []Answer) []Decision {
	asked := map[string]string{}
	for _, n := range in.Names {
		asked[models.MerchantGuessKey(n)] = n
	}
	byName := map[string]string{}
	for _, m := range in.Merchants {
		for _, a := range m.Aliases {
			if k := models.MerchantGuessKey(a); k != "" {
				byName[k] = m.ID
			}
		}
	}
	// names win over aliases
	for _, m := range in.Merchants {
		byName[models.MerchantGuessKey(m.Name)] = m.ID
	}

	var out []Decision
	seen := map[string]bool{}
	for _, a := range answers {
		key := models.MerchantGuessKey(a.Raw)
		raw, ok := asked[key]
		if !ok || seen[key] {
			continue
		}
		d := Decision{Raw: raw}
		if i := refIndex(a.MatchRef, "M", len(in.Merchants)); i >= 0 {
			d.MatchID = in.Merchants[i].ID
		} else {
			name := strings.Join(strings.Fields(a.NewName), " ")
			if name == "" || utf8.RuneCountInString(name) > MaxNameLen {
				continue
			}
			if id, ok := byName[models.MerchantGuessKey(name)]; ok {
				d.MatchID = id
			} else {
				d.NewName = name
				if i := refIndex(a.CategoryRef, "C", len(in.Categories)); i >= 0 {
					d.CategoryID = in.Categories[i].ID
				}
				d.Labels = cleanLabels(a.Labels)
			}
		}
		seen[key] = true
		out = append(out, d)
	}
	return out
}

func cleanLabels(labels []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range labels {
		l = strings.Join(strings.Fields(l), " ")
		k := strings.ToLower(l)
		if l == "" || utf8.RuneCountInString(l) > 40 || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, l)
		if len(out) == MaxLabels {
			break
		}
	}
	return out
}
