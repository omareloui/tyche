// Package txguess has Gemini read a free text ("coffee 85 cib yesterday,
// uber 120", "lent Ahmed 500") into transactions (quick add). The user's
// accounts, categories and most used counterparties are sent by short refs
// (A<i>, C<i>, P<i>), never ids; the answers are checked by Parse.
package txguess

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/omareloui/tyche/internal/gemini"
	"github.com/omareloui/tyche/internal/models"
)

// MaxTxs is the most transactions a text makes.
const MaxTxs = 10

// MaxTextLen is the longest text read (in runes).
const MaxTextLen = 1000

// MaxCounterparties is how many counterparties (the most used) are sent.
const MaxCounterparties = 100

// MaxNameLen is the longest name of a new counterparty, longer ones dropped.
const MaxNameLen = 60

// maxDescriptionLen and maxNoteLen cap what the model writes.
const (
	maxDescriptionLen = 120
	maxNoteLen        = 500
)

// Kind is what a guessed transaction is, as the model answers it: the
// transaction kinds, debts split by their direction.
type Kind string

const (
	KindExpense  Kind = "expense"
	KindIncome   Kind = "income"
	KindRefund   Kind = "refund"
	KindTransfer Kind = "transfer"
	// KindLend is money going to a person: lent, or paid back to them.
	KindLend Kind = "lend"
	// KindBorrow is money coming from a person: borrowed, or paid back by
	// them.
	KindBorrow Kind = "borrow"
)

// TxKind is the transaction kind of k.
func (k Kind) TxKind() models.TxKind {
	switch k {
	case KindExpense:
		return models.ExpenseTxKind
	case KindIncome:
		return models.IncomeTxKind
	case KindRefund:
		return models.RefundTxKind
	case KindTransfer:
		return models.TransferTxKind
	case KindLend, KindBorrow:
		return models.DebtTxKind
	}
	return models.NilTxKind
}

// IsDebt tells k is a debt with a person.
func (k Kind) IsDebt() bool { return k == KindLend || k == KindBorrow }

// Account is one of the user's accounts.
type Account struct {
	ID       string
	Name     string
	Currency string
	Kind     string
}

// Category is one of the user's categories, with the kinds it takes.
type Category struct {
	ID    string
	Name  string
	Kinds []models.TxKind
}

func (c *Category) takes(k models.TxKind) bool {
	for _, ck := range c.Kinds {
		if ck == k.CategoryKind() {
			return true
		}
	}
	return false
}

// Counterparty is one of the user's counterparties.
type Counterparty struct {
	ID         string
	Name       string
	IsMerchant bool
	Aliases    []string
}

// In is what's asked: the text, with what its transactions can be.
type In struct {
	Text string
	// Today is the user's day, Timezone its zone's name.
	Today    time.Time
	Timezone string
	// DefaultAccount is the account's name used when the text names none,
	// DefaultCurrency the user's main currency.
	DefaultAccount  string
	DefaultCurrency string
	Accounts        []Account
	Categories      []Category
	Counterparties  []Counterparty
}

// Answer is a transaction as the model answered it, unchecked.
type Answer struct {
	Kind            string  `json:"kind"`
	Amount          float64 `json:"amount"`
	Currency        string  `json:"currency"`
	Grams           bool    `json:"grams"`
	Account         string  `json:"account"`
	ToAccount       string  `json:"to_account"`
	Category        string  `json:"category"`
	Counterparty    string  `json:"counterparty"`
	NewCounterparty string  `json:"new_counterparty"`
	NewIsMerchant   bool    `json:"new_is_merchant"`
	Date            string  `json:"date"`
	Time            string  `json:"time"`
	Description     string  `json:"description"`
	Note            string  `json:"note"`
	// Tags are the text's #tags written with it, without the #.
	Tags []string `json:"tags"`
	// Text is the part of the text it was read from, to tell a skipped one.
	Text string `json:"text"`
}

// Guess is a checked Answer: ids among the ones sent, fields that fit its
// kind.
type Guess struct {
	Kind   Kind
	Amount float64
	// Currency is a code (upper case), "" for the account's.
	Currency string
	// Grams tells a metal's amount is in grams (else troy ounces).
	Grams       bool
	AccountID   string
	ToAccountID string
	CategoryID  string
	// CounterpartyID is the merchant / person, or NewCounterparty one to
	// create (NewIsMerchant: a business). A debt's is the person.
	CounterpartyID  string
	NewCounterparty string
	NewIsMerchant   bool
	// Day is the day (midnight UTC), zero for today; HasTime tells Clock
	// (since midnight) was told.
	Day         time.Time
	HasTime     bool
	Clock       time.Duration
	Description string
	Note        string
	// Tags are tag names, from the text's #tags only.
	Tags []string
	Text string
}

// Skipped is an answer left out, with why.
type Skipped struct {
	Text   string
	Reason string
}

// Guesser reads a text (an interface so tests fake it).
type Guesser interface {
	Guess(ctx context.Context, in In) ([]Answer, gemini.Usage, error)
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
		"transactions": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":             map[string]any{"type": "string", "enum": []string{string(KindExpense), string(KindIncome), string(KindRefund), string(KindTransfer), string(KindLend), string(KindBorrow)}},
					"amount":           map[string]any{"type": "number"},
					"currency":         map[string]any{"type": "string"},
					"grams":            map[string]any{"type": "boolean"},
					"account":          map[string]any{"type": "string"},
					"to_account":       map[string]any{"type": "string"},
					"category":         map[string]any{"type": "string"},
					"counterparty":     map[string]any{"type": "string"},
					"new_counterparty": map[string]any{"type": "string"},
					"new_is_merchant":  map[string]any{"type": "boolean"},
					"date":             map[string]any{"type": "string"},
					"time":             map[string]any{"type": "string"},
					"description":      map[string]any{"type": "string"},
					"note":             map[string]any{"type": "string"},
					"tags":             map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"text":             map[string]any{"type": "string"},
				},
				"required": []string{"kind", "amount", "account", "category", "counterparty", "new_counterparty", "date", "description", "text"},
			},
		},
	},
	"required": []string{"transactions"},
}

// Guess asks the model, unchecked (see Parse).
func (c *Client) Guess(ctx context.Context, in In) ([]Answer, gemini.Usage, error) {
	ans, usage, err := c.Generate(ctx, []gemini.Part{{Text: Prompt(in)}}, responseSchema)
	if err != nil {
		return nil, usage, err
	}
	var a struct {
		Transactions []Answer `json:"transactions"`
	}
	if err := json.Unmarshal([]byte(ans.Text), &a); err != nil {
		return nil, usage, fmt.Errorf("unreadable answer (finish reason %s)", ans.FinishReason)
	}
	return a.Transactions, usage, nil
}

func accountRef(i int) string      { return "A" + strconv.Itoa(i+1) }
func categoryRef(i int) string     { return "C" + strconv.Itoa(i+1) }
func counterpartyRef(i int) string { return "P" + strconv.Itoa(i+1) }

// refIndex is the index of a "<prefix><i>" ref of n items, -1 when it's
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

// Prompt asks to read the text into transactions, the user's accounts,
// categories and counterparties by refs.
func Prompt(in In) string {
	var b strings.Builder
	fmt.Fprintf(&b, `You read what a user of a personal finance app typed (or said) to add transactions, in English, Arabic or a mix, e.g. "coffee 85 cib yesterday, lunch 200 cash", "lent Ahmed 500", "salary 40k to hsbc", "قهوة ٨٥". Answer one transaction per amount spent, earned or moved, at most %d.

Today is %s (%s), in the time zone %s.

For each transaction:
- "kind": expense (spent), income (earned, salary, a gift received), refund (money back from a business for something bought), transfer (between two of the user's accounts), lend (money given to a person: lent to them or paid back to them), borrow (money from a person: borrowed from them or them paying the user back).
- "amount": positive, as a number ("40k" = 40000, "1.5m" = 1500000). "currency": its ISO 4217 code when the text tells one ("$" = USD, "€" = EUR, "جنيه" = EGP), else empty (the account's). For gold, silver, platinum or palladium (XAU, XAG, XPT, XPD), "grams" true when the amount is in grams.
- "account": the ref of the account the money leaves (expense, transfer, lend) or comes into (income, refund, borrow), when the text names it (by name, bank or kind, e.g. "cash", "cib", "visa"); else empty (the default account, %s). "to_account": a transfer's receiving account's ref.
- "category": the ref of the category that fits best among the ones taking the kind (empty when none fits; never on transfers, lend or borrow).
- "counterparty": the ref of the business or person the text names (expense, income, refund) or the person of a lend / borrow; when none of the user's is them, "counterparty" empty and "new_counterparty" a clean name for them (title case, as people call it), "new_is_merchant" true for a business, false for a person. Both empty when the text names nobody (e.g. "coffee 85": no counterparty, the category tells what it was). Never a counterparty on a transfer.
- "date": YYYY-MM-DD, worked out from today ("yesterday", "last friday", "on the 3rd"); today's when the text doesn't tell. "time": HH:MM (24h) only when the text tells one, else empty.
- "description": a short description (a few words, the text's language) of what it was, e.g. "Coffee", "Lunch"; empty when the category and counterparty say it all. "note": anything else worth keeping the text told, else empty.
- "tags": the text's #words (e.g. "#work", "#دبي") written with the transaction, without the "#"; a # standing apart from any transaction (e.g. at the end) goes on every one. Never put a #word in the description or note, never make up tags.
- "text": the part of the text the transaction was read from, as typed.
Only refs listed below are valid. Leave out what isn't a transaction (no amount).

`, MaxTxs, in.Today.Format(time.DateOnly), in.Today.Weekday(), in.Timezone, nonEmpty(in.DefaultAccount, "none"))

	fmt.Fprintf(&b, "The user's main currency: %s\n", nonEmpty(in.DefaultCurrency, "unknown"))
	b.WriteString("\nAccounts (ref: name, currency, kind):\n")
	if len(in.Accounts) == 0 {
		b.WriteString("(none)\n")
	}
	for i, a := range in.Accounts {
		fmt.Fprintf(&b, "%s: %s, %s, %s\n", accountRef(i), a.Name, a.Currency, a.Kind)
	}
	b.WriteString("\nCategories (ref: name, kinds it takes):\n")
	if len(in.Categories) == 0 {
		b.WriteString("(none)\n")
	}
	for i, c := range in.Categories {
		kinds := make([]string, 0, len(c.Kinds))
		for _, k := range c.Kinds {
			kinds = append(kinds, strings.ToLower(k.String()))
		}
		fmt.Fprintf(&b, "%s: %s (%s)\n", categoryRef(i), c.Name, strings.Join(kinds, ", "))
	}
	b.WriteString("\nCounterparties (ref: name, business or person, then aliases):\n")
	if len(in.Counterparties) == 0 {
		b.WriteString("(none)\n")
	}
	for i, cp := range in.Counterparties {
		what := "person"
		if cp.IsMerchant {
			what = "business"
		}
		fmt.Fprintf(&b, "%s: %s, %s", counterpartyRef(i), cp.Name, what)
		if len(cp.Aliases) > 0 {
			fmt.Fprintf(&b, " | %s", strings.Join(cp.Aliases, " | "))
		}
		b.WriteByte('\n')
	}
	b.WriteString("\nText:\n")
	b.WriteString(in.Text)
	b.WriteByte('\n')
	return b.String()
}

func nonEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// nameKey is a name compared any case, spaces folded.
func nameKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// Parse checks the answers against what was sent: refs among the ones sent
// (others dropped), fields that don't fit the kind dropped (a category of
// another kind, a counterparty on a transfer), a new counterparty named
// like one of the user's (name or alias, any case) taken as it. An answer
// without a kind, an amount, a transfer's other account or a debt's person
// is skipped, told why. At most MaxTxs.
func Parse(in In, answers []Answer) ([]Guess, []Skipped) {
	byName := map[string]string{}
	isMerchant := map[string]bool{}
	for _, cp := range in.Counterparties {
		isMerchant[cp.ID] = cp.IsMerchant
		for _, a := range cp.Aliases {
			if k := nameKey(a); k != "" {
				byName[k] = cp.ID
			}
		}
	}
	// names win over aliases
	for _, cp := range in.Counterparties {
		byName[nameKey(cp.Name)] = cp.ID
	}

	tags := HashTags(in.Text)
	byTag := map[string]string{}
	for _, t := range tags {
		byTag[strings.ToLower(t)] = t
	}
	tagged := false

	var out []Guess
	var skipped []Skipped
	for _, a := range answers {
		text := clip(strings.TrimSpace(a.Text), 200)
		skip := func(reason string) { skipped = append(skipped, Skipped{Text: text, Reason: reason}) }
		if len(out) == MaxTxs {
			skip(fmt.Sprintf("Over %d transactions at once", MaxTxs))
			continue
		}
		g := Guess{Kind: Kind(strings.ToLower(strings.TrimSpace(a.Kind))), Text: text}
		kind := g.Kind.TxKind()
		if kind == models.NilTxKind {
			skip("Couldn't tell what kind of transaction it is")
			continue
		}
		if !(a.Amount > 0) || a.Amount > 1e12 {
			skip("No amount")
			continue
		}
		g.Amount = a.Amount
		if cur := strings.ToUpper(strings.TrimSpace(a.Currency)); len(cur) == 3 && isLetters(cur) {
			g.Currency = cur
		}
		g.Grams = a.Grams

		if i := refIndex(a.Account, "A", len(in.Accounts)); i >= 0 {
			g.AccountID = in.Accounts[i].ID
		}
		if kind == models.TransferTxKind {
			i := refIndex(a.ToAccount, "A", len(in.Accounts))
			if i < 0 || in.Accounts[i].ID == g.AccountID {
				skip("A transfer needs the account it goes to")
				continue
			}
			g.ToAccountID = in.Accounts[i].ID
		}

		if kind != models.TransferTxKind && !g.Kind.IsDebt() {
			if i := refIndex(a.Category, "C", len(in.Categories)); i >= 0 && in.Categories[i].takes(kind) {
				g.CategoryID = in.Categories[i].ID
			}
		}
		if kind != models.TransferTxKind {
			if i := refIndex(a.Counterparty, "P", len(in.Counterparties)); i >= 0 {
				g.CounterpartyID = in.Counterparties[i].ID
			} else if name := strings.Join(strings.Fields(a.NewCounterparty), " "); name != "" && utf8.RuneCountInString(name) <= MaxNameLen {
				if id, ok := byName[nameKey(name)]; ok {
					g.CounterpartyID = id
				} else {
					g.NewCounterparty, g.NewIsMerchant = name, a.NewIsMerchant
				}
			}
			if g.Kind.IsDebt() {
				// a debt is with a person
				if g.CounterpartyID != "" && isMerchant[g.CounterpartyID] {
					g.CounterpartyID = ""
				}
				g.NewIsMerchant = false
				if g.CounterpartyID == "" && g.NewCounterparty == "" {
					skip("Couldn't tell who the debt is with")
					continue
				}
			}
		}

		if d, err := time.Parse(time.DateOnly, strings.TrimSpace(a.Date)); err == nil {
			g.Day = d
		}
		if c, ok := parseClock(a.Time); ok {
			g.HasTime, g.Clock = true, c
		}
		g.Description = clip(strings.Join(strings.Fields(StripHashTags(a.Description)), " "), maxDescriptionLen)
		g.Note = clip(strings.TrimSpace(StripHashTags(a.Note)), maxNoteLen)
		for _, t := range a.Tags {
			name, ok := byTag[strings.ToLower(strings.TrimPrefix(strings.TrimSpace(t), "#"))]
			if ok && !slices.Contains(g.Tags, name) {
				g.Tags = append(g.Tags, name)
				tagged = true
			}
		}
		out = append(out, g)
	}
	// #tags the model put on none go on every one
	if !tagged && len(tags) > 0 {
		for i := range out {
			out[i].Tags = tags
		}
	}
	return out, skipped
}

// hashTagRe is a #tag: letters, digits, _ and - after the #, not inside a
// word (an "a#b" isn't one).
var hashTagRe = regexp.MustCompile(`(^|[^\pL\pN_#])#([\pL\pN][\pL\pN_-]*)`)

// HashTags are the text's #tags (without the #), each once (any case), the
// ones over models.MaxTagNameLen left out, at most models.MaxTxTags.
func HashTags(text string) []string {
	var out []string
	for _, m := range hashTagRe.FindAllStringSubmatch(text, -1) {
		name := strings.TrimRight(m[2], "-_")
		if name == "" || utf8.RuneCountInString(name) > models.MaxTagNameLen ||
			slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, name) }) {
			continue
		}
		out = append(out, name)
		if len(out) == models.MaxTxTags {
			break
		}
	}
	return out
}

// StripHashTags drops the text's #tags.
func StripHashTags(text string) string {
	return strings.Join(strings.Fields(hashTagRe.ReplaceAllString(text, "$1")), " ")
}

func parseClock(s string) (time.Duration, bool) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute, true
}

func isLetters(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:n]))
}

// NormalizeDigits turns Arabic-Indic and Persian digits (and the Arabic
// decimal / thousands separators) into ASCII ones, so "قهوة ٨٥" reads 85.
func NormalizeDigits(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= '٠' && r <= '٩':
			return '0' + (r - '٠')
		case r >= '۰' && r <= '۹':
			return '0' + (r - '۰')
		case r == '٫':
			return '.'
		case r == '٬':
			return ','
		}
		return r
	}, s)
}

// Clean is the text as read: digits normalized, trimmed, cut to MaxTextLen.
func Clean(text string) string {
	return clip(strings.TrimSpace(NormalizeDigits(text)), MaxTextLen)
}

// Cheap reads, without the model, a text that's only an amount ("85") or a
// name and an amount ("coffee 85", "Goods Shop 120"), #tags aside: the name a
// counterparty's (name or alias) or a category's, any case. An expense of
// the default account, unless the category only takes income. False when
// the text is anything else.
func Cheap(in In, text string) (Guess, bool) {
	fields := strings.Fields(StripHashTags(NormalizeDigits(text)))
	if len(fields) == 0 {
		return Guess{}, false
	}
	amount, ok := parseAmount(fields[len(fields)-1])
	if !ok {
		return Guess{}, false
	}
	g := Guess{Kind: KindExpense, Amount: amount, Tags: HashTags(text), Text: strings.TrimSpace(text)}
	name := nameKey(strings.Join(fields[:len(fields)-1], " "))
	if name == "" {
		return g, true
	}
	for _, cp := range in.Counterparties {
		if nameKey(cp.Name) == name {
			g.CounterpartyID = cp.ID
			return g, true
		}
	}
	for _, cp := range in.Counterparties {
		for _, a := range cp.Aliases {
			if nameKey(a) == name {
				g.CounterpartyID = cp.ID
				return g, true
			}
		}
	}
	for _, c := range in.Categories {
		if nameKey(c.Name) != name {
			continue
		}
		switch {
		case c.takes(models.ExpenseTxKind):
		case c.takes(models.IncomeTxKind):
			g.Kind = KindIncome
		default:
			return Guess{}, false
		}
		g.CategoryID = c.ID
		return g, true
	}
	return Guess{}, false
}

// parseAmount reads a positive amount: digits with "," thousands and a "."
// decimal, "k" for thousands.
func parseAmount(s string) (float64, bool) {
	mult := 1.0
	if strings.HasSuffix(s, "k") || strings.HasSuffix(s, "K") {
		mult, s = 1000, s[:len(s)-1]
	}
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) && r != ',' && r != '.' {
			return 0, false
		}
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	if err != nil || !(v > 0) {
		return 0, false
	}
	return v * mult, true
}
