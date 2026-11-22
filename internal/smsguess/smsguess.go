// Package smsguess has Gemini guess the template of a bank sms no template
// reads, in the smsparser syntax. A guess is only kept when it reads the sms
// (Verify); it then prefills the template form, the user saving it.
package smsguess

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/omareloui/tyche/internal/gemini"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

// MaxExamples is how many known templates are shown to the model.
const MaxExamples = 8

// ErrNoGuess is a guess that doesn't read the sms (after the retry).
var ErrNoGuess = errors.New("couldn't guess a template reading the sms")

// In is an sms to guess the template of.
type In struct {
	Sender string
	Body   string
	// Examples are known templates (with their samples), see PickExamples.
	Examples []models.SMSTemplate
	// Previous is an earlier guess that didn't read the sms, PreviousErr why.
	Previous    *models.SMSTemplateGuess
	PreviousErr error
}

// Guesser guesses an sms' template (an interface so tests fake it).
type Guesser interface {
	Guess(ctx context.Context, in In) (*models.SMSTemplateGuess, gemini.Usage, error)
	Model() string
}

// Client guesses with Gemini.
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

var kindNames = map[string]models.TxKind{
	"expense":  models.ExpenseTxKind,
	"income":   models.IncomeTxKind,
	"transfer": models.TransferTxKind,
	"refund":   models.RefundTxKind,
}

func kindName(k models.TxKind) string {
	for name, kind := range kindNames {
		if kind == k {
			return name
		}
	}
	return ""
}

var responseSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"action":  map[string]any{"type": "string", "enum": []string{"transaction", "ignore"}},
		"tx_kind": map[string]any{"type": "string", "enum": []string{"expense", "income", "transfer", "refund", ""}},
		"name":    map[string]any{"type": "string"},
		"bank":    map[string]any{"type": "string"},
		"body":    map[string]any{"type": "string"},
		"fixed": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"field": map[string]any{"type": "string", "enum": models.SMSTemplateFixedFields},
					"value": map[string]any{"type": "string"},
				},
				"required": []string{"field", "value"},
			},
		},
	},
	"required": []string{"action", "tx_kind", "name", "bank", "body", "fixed"},
}

type answer struct {
	Action string `json:"action"`
	TxKind string `json:"tx_kind"`
	Name   string `json:"name"`
	Bank   string `json:"bank"`
	Body   string `json:"body"`
	Fixed  []struct {
		Field string `json:"field"`
		Value string `json:"value"`
	} `json:"fixed"`
}

func (a *answer) guess() *models.SMSTemplateGuess {
	g := &models.SMSTemplateGuess{
		Name:   strings.TrimSpace(a.Name),
		Bank:   strings.TrimSpace(a.Bank),
		Action: models.SMSTemplateAction(a.Action),
		Body:   strings.TrimSpace(a.Body),
	}
	if g.Action != models.SMSTemplateIgnore {
		g.Action = models.SMSTemplateTransaction
		g.TxKind = kindNames[strings.ToLower(strings.TrimSpace(a.TxKind))]
		for _, f := range a.Fixed {
			if v := strings.TrimSpace(f.Value); v != "" && slices.Contains(models.SMSTemplateFixedFields, f.Field) {
				if g.Fixed == nil {
					g.Fixed = map[string]string{}
				}
				g.Fixed[f.Field] = v
			}
		}
	}
	return g
}

// Guess asks the model for the sms' template, unverified.
func (c *Client) Guess(ctx context.Context, in In) (*models.SMSTemplateGuess, gemini.Usage, error) {
	ans, usage, err := c.Generate(ctx, []gemini.Part{{Text: Prompt(in)}}, responseSchema)
	if err != nil {
		return nil, usage, err
	}
	var a answer
	if err := json.Unmarshal([]byte(ans.Text), &a); err != nil {
		return nil, usage, fmt.Errorf("%w: unreadable answer (finish reason %s)", ErrNoGuess, ans.FinishReason)
	}
	return a.guess(), usage, nil
}

// Verify tells whether the guess reads the sms: it compiles, matches the
// whole body and, for a transaction, reads an amount.
func Verify(g *models.SMSTemplateGuess, body string) error {
	if g.Body == "" {
		return errors.New("the template is empty")
	}
	if g.Action == models.SMSTemplateTransaction && kindName(g.TxKind) == "" {
		return errors.New("tx_kind must be expense, income, transfer or refund")
	}
	t := g.Template("", body)
	m, err := smsparser.MatchTemplate(*t, body)
	if g.Action == models.SMSTemplateIgnore {
		if errors.Is(err, smsparser.ErrNotTransaction) {
			return nil
		}
		return fmt.Errorf("the template doesn't match the sms: %v", err)
	}
	if err != nil {
		return fmt.Errorf("the template doesn't read the sms: %v", err)
	}
	if m.SMS.Amount <= 0 {
		return errors.New("the template reads no amount")
	}
	return nil
}

// GuessVerified guesses, retrying once with why the first guess didn't read
// the sms. The usage adds up every call.
func GuessVerified(ctx context.Context, g Guesser, in In) (*models.SMSTemplateGuess, gemini.Usage, error) {
	var total gemini.Usage
	for range 2 {
		guess, usage, err := g.Guess(ctx, in)
		total.InputTokens += usage.InputTokens
		total.OutputTokens += usage.OutputTokens
		if err != nil && !errors.Is(err, ErrNoGuess) {
			return nil, total, err
		}
		if err == nil {
			verr := Verify(guess, in.Body)
			if verr == nil {
				return guess, total, nil
			}
			in.Previous, in.PreviousErr = guess, verr
			continue
		}
		in.Previous, in.PreviousErr = nil, err
	}
	return nil, total, fmt.Errorf("%w: %v", ErrNoGuess, in.PreviousErr)
}

// PickExamples picks up to MaxExamples templates to show: the sender's
// first, then one of each action and kind not shown yet, then the rest, in
// order. Only the ones with a sample are shown.
func PickExamples(sender string, templates []models.SMSTemplate) []models.SMSTemplate {
	var out []models.SMSTemplate
	picked := map[string]bool{}
	add := func(t models.SMSTemplate) {
		if len(out) < MaxExamples && !picked[t.ID+t.Body] && t.Sample != "" && t.IsEnabled {
			picked[t.ID+t.Body] = true
			out = append(out, t)
		}
	}
	for _, t := range templates {
		if t.AppliesTo(sender) {
			add(t)
		}
	}
	seen := map[string]bool{}
	for _, t := range out {
		seen[string(t.Action)+kindName(t.TxKind)] = true
	}
	for _, t := range templates {
		if k := string(t.Action) + kindName(t.TxKind); !seen[k] {
			seen[k] = true
			add(t)
		}
	}
	for _, t := range templates {
		add(t)
	}
	return out
}

// Prompt is what the model is asked.
func Prompt(in In) string {
	var b strings.Builder
	b.WriteString(`You write templates reading bank SMS (Arabic, English or both) for a personal finance app.
A template is the SMS's text with its variable parts replaced by placeholders between {{ and }}. Its text has to match the WHOLE SMS: any run of whitespace in it matches any run of whitespace in the SMS, and placeholders ignore the whitespace around them. Placeholders:
- {{field}}: a field read from the SMS (fields below)
- {{counterparty:merchant}}: the counterparty, when it's a shop / merchant (a plain {{counterparty}} for a person)
- {{date:DD/MM/YYYY}}: a date in that format (tokens DD, MM, MMM, YY, YYYY); a plain {{date}} detects day-first dates like 13/05/26, 07-11-2025, 07OCT25, 07 DEC 2025
- {{field:/regexp/}}: the field, matching the Go regexp instead of its default
- {{/regexp/}}: text to skip, matching the Go regexp
- {{*}}: any text to skip, e.g. the end of the SMS (a marketing line, a call centre number)
Fields (name: default regexp, what it is):
`)
	for _, f := range smsparser.Fields() {
		fmt.Fprintf(&b, "- %s: %s, %s\n", f.Name, f.Default, f.Desc)
	}
	b.WriteString(`
Rules:
- Replace every part that changes from one SMS to the next (amounts, card / account numbers, merchants, people, dates, times, references, balances) by its placeholder or a skip; keep the bank's fixed wording as literal text, so the template reads the next SMS of the same kind.
- action "transaction" for money moved: tx_kind "expense" (a purchase, a payment, money out to someone), "income" (money in: salary, a transfer received), "transfer" (between the user's own accounts, e.g. an ATM withdrawal to Cash, needs to_account_id read or fixed), "refund" (a merchant's money back, a reversal of a purchase). account_id must be read (the card / account the SMS is about) or fixed.
- action "ignore" (tx_kind "") for SMS that move no money: OTPs, declined or failed payments, statements, balance alerts, promotions.
- fixed sets fields to constants instead of reading them, only ` + strings.Join(models.SMSTemplateFixedFields, ", ") + `, e.g. to_account_id = Cash for an ATM withdrawal. Leave it empty otherwise.
- name: a short description of this kind of SMS, e.g. "Credit card purchase"; bank: the bank's name, empty when unknown.
`)
	if len(in.Examples) > 0 {
		b.WriteString("\nKnown templates, as examples:\n")
		for _, t := range in.Examples {
			ex := map[string]any{"sender": strings.Join(t.Senders, ", "), "sms": t.Sample, "action": t.Action, "tx_kind": kindName(t.TxKind), "name": t.Name, "bank": t.Bank, "body": t.Body}
			if len(t.Fixed) > 0 {
				ex["fixed"] = t.Fixed
			}
			raw, _ := json.Marshal(ex)
			b.Write(raw)
			b.WriteByte('\n')
		}
	}
	fmt.Fprintf(&b, "\nWrite the template of this SMS.\nSender: %s\nSMS:\n%s\n", cmp.Or(strings.TrimSpace(in.Sender), "(unknown)"), in.Body)
	if in.Previous != nil || in.PreviousErr != nil {
		b.WriteString("\nAn earlier answer didn't work")
		if in.Previous != nil {
			raw, _ := json.Marshal(map[string]any{"action": in.Previous.Action, "tx_kind": kindName(in.Previous.TxKind), "body": in.Previous.Body, "fixed": in.Previous.Fixed})
			fmt.Fprintf(&b, ":\n%s\n", raw)
		}
		if in.PreviousErr != nil {
			fmt.Fprintf(&b, "\nWhy: %v\n", in.PreviousErr)
		}
		b.WriteString("Fix it.\n")
	}
	return b.String()
}
