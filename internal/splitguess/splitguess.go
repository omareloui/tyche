// Package splitguess has Gemini read who had what of a split bill from a
// sentence ("Ahmed had the burger and fries, Sara and me shared the pizza,
// drinks for everyone") into the items' shares. Only the items' names and
// quantities, the names of the people in the split and the sentence are
// sent: never amounts, ids or the merchant.
package splitguess

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/omareloui/tyche/internal/gemini"
	"github.com/omareloui/tyche/internal/models"
)

// MaxTextLen is the longest sentence read, longer ones refused.
const MaxTextLen = 500

// MaxNameLen is the longest name of a new person, longer ones dropped.
const MaxNameLen = 60

// MaxNewPeople is the most people a sentence adds to the split.
const MaxNewPeople = 10

// Line is an item of the bill (charges aren't sent: they follow the
// items).
type Line struct {
	ID       string
	Name     string
	Quantity float64
}

// Person is someone already in the split.
type Person struct {
	ID   string
	Name string
}

// In is what's asked: the sentence, with the bill's items and the people in
// the split.
type In struct {
	Lines  []Line
	People []Person
	Text   string
}

// Taker is someone in an Answer's item: "me", a person's ref ("P<i>") or a
// new person's name, with their units of it (0 = not said).
type Taker struct {
	Who   string `json:"who"`
	Units int    `json:"units"`
}

// AnswerShare is who had an item ("I<i>"), as answered.
type AnswerShare struct {
	Item     string  `json:"item"`
	Everyone bool    `json:"everyone"`
	Takers   []Taker `json:"people"`
}

// Answer is the model's reading of the sentence, unchecked (see Parse).
type Answer struct {
	Shares    []AnswerShare `json:"shares"`
	NewPeople []string      `json:"new_people"`
}

// Out is a checked Answer: the items' shares by line id, their people
// models.SplitMe, the ids of the people in the split or NewRef(i) for
// NewPeople[i] (to resolve to a counterparty); the items not mentioned are
// left out.
type Out struct {
	Shares    map[string]models.SplitItemShare
	NewPeople []string
}

// NewRef is the person id standing for Out.NewPeople[i] until it's a
// counterparty.
func NewRef(i int) string { return "new:" + strconv.Itoa(i) }

// Guesser reads the sentence (an interface so tests fake it).
type Guesser interface {
	Guess(ctx context.Context, in In) (Answer, gemini.Usage, error)
	Model() string
}

// Client reads with Gemini.
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
		"shares": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"item":     map[string]any{"type": "string"},
					"everyone": map[string]any{"type": "boolean"},
					"people": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"who":   map[string]any{"type": "string"},
								"units": map[string]any{"type": "integer"},
							},
							"required": []string{"who"},
						},
					},
				},
				"required": []string{"item", "everyone", "people"},
			},
		},
		"new_people": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	},
	"required": []string{"shares", "new_people"},
}

// Guess asks the model, unchecked (see Parse).
func (c *Client) Guess(ctx context.Context, in In) (Answer, gemini.Usage, error) {
	ans, usage, err := c.Generate(ctx, []gemini.Part{{Text: Prompt(in)}}, responseSchema)
	if err != nil {
		return Answer{}, usage, err
	}
	var a Answer
	if err := json.Unmarshal([]byte(ans.Text), &a); err != nil {
		return Answer{}, usage, fmt.Errorf("unreadable answer (finish reason %s)", ans.FinishReason)
	}
	return a, usage, nil
}

// Clean is the sentence with its whitespace folded.
func Clean(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// TooLong tells the sentence is over MaxTextLen characters.
func TooLong(text string) bool {
	return utf8.RuneCountInString(Clean(text)) > MaxTextLen
}

func itemRef(i int) string   { return "I" + strconv.Itoa(i+1) }
func personRef(i int) string { return "P" + strconv.Itoa(i+1) }

// refIndex is the index of a "<prefix><i>" ref of n items, -1 when it's none
// of them.
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

func qty(q float64) string { return strconv.FormatFloat(q, 'f', -1, 64) }

// Prompt asks to read the sentence into the items' shares, the items and
// people referred to by short refs (an invented one is told apart).
func Prompt(in In) string {
	var b strings.Builder
	b.WriteString(`You help split a restaurant or shop bill between the user ("me") and other people. The user describes, in their own words and possibly in any language, who had what. Read it into who had each item of the bill.

Rules:
- Answer one share per item the sentence mentions, by its ref in "item" (e.g. "I2"). Leave out the items it doesn't mention; never invent items.
- An item had by everyone at the table ("for everyone", "we all shared", "split evenly"): "everyone" true and "people" empty.
- Else list who had it in "people": "me" for the user (I, me, my, mine, we when the user is among them), a person's ref (e.g. "P1") for someone below, or the person's name as said for someone not below.
- An item with a quantity over 1 is shared by its units: give each one's "units" (how many of the quantity they had, whole numbers, adding up to at most the quantity). "Sara and me shared 2 pizzas" → 1 each; "the burgers were Ahmed's" → all of them his. For an item with a quantity of 1, units are 0.
- "new_people" lists, once each, the names of the people said who aren't below, written as said, capitalized. Never the user.
- Match the items by meaning, loosely (singular/plural, other spelling or language, "drinks" covering every drink line).

`)
	b.WriteString("Items (ref: name × quantity):\n")
	for i, l := range in.Lines {
		fmt.Fprintf(&b, "%s: %s × %s\n", itemRef(i), l.Name, qty(l.Quantity))
	}
	b.WriteString("\nPeople in the split (ref: name):\n")
	if len(in.People) == 0 {
		b.WriteString("(none yet)\n")
	}
	for i, p := range in.People {
		fmt.Fprintf(&b, "%s: %s\n", personRef(i), p.Name)
	}
	b.WriteString("\nWhat the user said:\n")
	b.WriteString(in.Text)
	b.WriteByte('\n')
	return b.String()
}

// nameKey is a name compared loosely: lowercased, whitespace folded.
func nameKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// isMe tells the name stands for the user.
func isMe(s string) bool {
	switch nameKey(s) {
	case models.SplitMe, "i", "you", "myself", "user", "the user":
		return true
	}
	return false
}

// Parse checks the answer against what was asked: shares only of the lines
// sent (the first per line), people among the ones sent, the user or the new
// people (a new name like one of the people's is that person); an item by
// units (models.ItemByUnits) keeps the people's units up to its quantity,
// the user having the rest, and people without units share it as evenly as
// it goes; units of other items are dropped. Anything else is left out.
func Parse(in In, a Answer) Out {
	known := map[string]string{}
	for _, p := range in.People {
		known[nameKey(p.Name)] = p.ID
	}

	out := Out{Shares: map[string]models.SplitItemShare{}}
	newAt := map[string]int{}
	addNew := func(name string) string {
		name = strings.Join(strings.Fields(name), " ")
		k := nameKey(name)
		if k == "" || isMe(name) || utf8.RuneCountInString(name) > MaxNameLen {
			return ""
		}
		if id, ok := known[k]; ok {
			return id
		}
		if i, ok := newAt[k]; ok {
			return NewRef(i)
		}
		if len(out.NewPeople) >= MaxNewPeople {
			return ""
		}
		newAt[k] = len(out.NewPeople)
		out.NewPeople = append(out.NewPeople, name)
		return NewRef(newAt[k])
	}
	who := func(s string) string {
		if isMe(s) {
			return models.SplitMe
		}
		if i := refIndex(s, "P", len(in.People)); i >= 0 {
			return in.People[i].ID
		}
		if strings.TrimSpace(s) == "" || refIndex(s, "P", math.MaxInt) >= 0 || refIndex(s, "I", math.MaxInt) >= 0 {
			// an empty or invented ref
			return ""
		}
		return addNew(s)
	}

	// the names listed first keep the order said
	for _, n := range a.NewPeople {
		if !isMe(n) && refIndex(n, "P", math.MaxInt) < 0 {
			addNew(n)
		}
	}

	for _, sh := range a.Shares {
		i := refIndex(sh.Item, "I", len(in.Lines))
		if i < 0 {
			continue
		}
		line := in.Lines[i]
		if _, ok := out.Shares[line.ID]; ok {
			continue
		}
		if sh.Everyone {
			out.Shares[line.ID] = models.SplitItemShare{All: true}
			continue
		}
		var takers []Taker
		for _, t := range sh.Takers {
			id := who(t.Who)
			if id == "" || slices.ContainsFunc(takers, func(x Taker) bool { return x.Who == id }) {
				continue
			}
			takers = append(takers, Taker{Who: id, Units: t.Units})
		}
		if len(takers) == 0 {
			continue
		}
		if models.ItemByUnits(&models.TxItem{Quantity: line.Quantity}) {
			out.Shares[line.ID] = byUnits(takers, line.Quantity)
			continue
		}
		var with []models.ItemShare
		for _, t := range takers {
			with = append(with, models.ItemShare{Person: t.Who, Units: 1})
		}
		out.Shares[line.ID] = models.SplitItemShare{With: with}
	}
	return out
}

// byUnits shares an item of qty units: the people's units as said, capped
// to what's left; when none of them said any, the quantity spread as evenly
// as it goes over everyone named (the user's part the rest, implicitly).
func byUnits(takers []Taker, qty float64) models.SplitItemShare {
	said := slices.ContainsFunc(takers, func(t Taker) bool { return t.Units > 0 })
	total := int(qty)
	if !said {
		each, extra := total/len(takers), total%len(takers)
		for i := range takers {
			takers[i].Units = each
			if i < extra {
				takers[i].Units++
			}
		}
	}
	left := qty
	var with []models.ItemShare
	for _, t := range takers {
		if t.Who == models.SplitMe || t.Units <= 0 || left <= 0 {
			continue
		}
		u := min(float64(t.Units), left)
		left -= u
		with = append(with, models.ItemShare{Person: t.Who, Units: u})
	}
	return models.SplitItemShare{With: with}
}
