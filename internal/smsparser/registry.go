// Package smsparser reads bank sms using templates, see Compile for their
// syntax. The shared templates are seeded from Seeds.
package smsparser

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

var (
	ErrCantParse = errors.New("can't parse this text")
	// ErrNotTransaction is returned for sms matching an ignore template, like
	// statements and reminders.
	ErrNotTransaction = errors.New("not a transaction")
)

type SMS struct {
	Bank         string
	TxKind       models.TxKind
	Amount       float64
	ToAmount     float64
	AccountID    string
	ToAccountID  string
	Currency     string
	ToCurrency   string
	Ref          string
	Counterparty string
	// AutoCreateCounterparty creates Counterparty on ingest if no existing
	// counterparty (or alias) matches it.
	AutoCreateCounterparty bool
	// CounterpartyIsMerchant creates the counterparty as a merchant.
	CounterpartyIsMerchant   bool
	Note                     string
	AvailableBalance         float64
	AvailableBalanceCurrency string
	// Timestamp is the date in the sms, at its time if HasTime. Zero if the
	// sms has no (valid) date, year 0 if the date has no year.
	Timestamp time.Time
	HasTime   bool
}

// Match is the template an sms matched.
type Match struct {
	Template *models.SMSTemplate
	// Fields are the raw values read from the sms.
	Fields map[string]string
	// SMS is nil for ignore templates.
	SMS *SMS
}

type compiled struct {
	tmpl    models.SMSTemplate
	pattern *Pattern
}

// TemplateError is a template that didn't compile, it's left out.
type TemplateError struct {
	Template models.SMSTemplate
	Err      error
}

type Registry struct {
	templates []compiled
	errs      []TemplateError
}

// NewRegistry compiles the enabled templates, the groups are tried in order
// (e.g. the user's templates then the shared ones) and within each the
// higher priority ones first, then the older ones.
func NewRegistry(groups ...[]models.SMSTemplate) *Registry {
	r := &Registry{}
	for _, group := range groups {
		group = slices.Clone(group)
		slices.SortStableFunc(group, func(a, b models.SMSTemplate) int {
			if c := cmp.Compare(b.Priority, a.Priority); c != 0 {
				return c
			}
			return a.CreatedAt.Compare(b.CreatedAt)
		})
		for _, t := range group {
			if !t.IsEnabled {
				continue
			}
			p, err := Compile(t.Body)
			if err != nil {
				r.errs = append(r.errs, TemplateError{Template: t, Err: err})
				continue
			}
			r.templates = append(r.templates, compiled{tmpl: t, pattern: p})
		}
	}
	return r
}

// Errors are the templates that didn't compile.
func (r *Registry) Errors() []TemplateError {
	return r.errs
}

// Parse reads the sms with the first template matching it. Only the
// templates for the sender are tried, along with the ones for any sender,
// unless no template is for the sender, then every template is tried (banks
// sometimes send from short numbers).
//
// The match is returned with ErrNotTransaction for ignore templates.
func (r *Registry) Parse(sender, body string) (*Match, error) {
	body = strings.TrimSpace(body)

	forSender := slices.ContainsFunc(r.templates, func(c compiled) bool { return c.tmpl.AppliesTo(sender) })

	var buildErr error
	for i := range r.templates {
		c := &r.templates[i]
		if forSender && len(c.tmpl.Senders) > 0 && !c.tmpl.AppliesTo(sender) {
			continue
		}
		vals, ok := c.pattern.Match(body)
		if !ok {
			continue
		}

		m := &Match{Template: &c.tmpl, Fields: vals}
		if c.tmpl.Action == models.SMSTemplateIgnore {
			return m, ErrNotTransaction
		}
		sms, err := c.build(vals)
		if err != nil {
			// a later template may read it
			buildErr = cmp.Or(buildErr, fmt.Errorf("%w: template %q: %v", ErrCantParse, c.tmpl.Name, err))
			continue
		}
		m.SMS = sms
		return m, nil
	}

	if buildErr != nil {
		return nil, buildErr
	}
	return nil, fmt.Errorf("%w: no template matched", ErrCantParse)
}

// MatchTemplate reads the sms with a single template, whatever its senders
// and whether it's enabled.
func MatchTemplate(t models.SMSTemplate, body string) (*Match, error) {
	p, err := Compile(t.Body)
	if err != nil {
		return nil, err
	}
	t.IsEnabled = true
	r := &Registry{templates: []compiled{{tmpl: t, pattern: p}}}
	return r.Parse("", body)
}

func (c *compiled) build(vals map[string]string) (*SMS, error) {
	t := &c.tmpl
	sms := &SMS{Bank: t.Bank, TxKind: t.TxKind}

	get := func(field string) string {
		if v, ok := vals[field]; ok {
			return v
		}
		return t.Fixed[field]
	}
	amount := func(field string) (float64, error) {
		v := get(field)
		if v == "" {
			return 0, nil
		}
		f, err := amountStrToFloat(v)
		if err != nil {
			return 0, fmt.Errorf("invalid %s %q", field, v)
		}
		return f, nil
	}

	var err error
	if sms.Amount, err = amount("amount"); err != nil {
		return nil, err
	}
	if sms.ToAmount, err = amount("to_amount"); err != nil {
		return nil, err
	}
	if sms.AvailableBalance, err = amount("available_amount"); err != nil {
		return nil, err
	}

	sms.AccountID = normalizeAccount(get("account_id"))
	sms.ToAccountID = normalizeAccount(get("to_account_id"))
	sms.Currency = normalizeCurrency(get("currency"))
	sms.ToCurrency = normalizeCurrency(get("to_currency"))
	sms.AvailableBalanceCurrency = normalizeCurrency(get("available_currency"))
	sms.Ref = get("ref")
	sms.Note = get("note")

	sms.Counterparty = normalizeCounterparty(get("counterparty"))
	sms.AutoCreateCounterparty = sms.Counterparty != ""
	if ph := c.pattern.placeholder("counterparty"); ph != nil && ph.modifier == "merchant" {
		sms.CounterpartyIsMerchant = true
	}

	if v := get("date"); v != "" {
		layout := ""
		if ph := c.pattern.placeholder("date"); ph != nil {
			layout = ph.layout
		}
		if d, ok := parseDate(v, layout); ok {
			sms.Timestamp = d
			if clock, ok := parseClock(get("time")); ok {
				sms.Timestamp, sms.HasTime = d.Add(clock), true
			}
		}
	}

	if sms.TxKind == models.TransferTxKind {
		if sms.Amount == 0 {
			sms.Amount = sms.ToAmount
		}
		if sms.ToAmount == 0 {
			sms.ToAmount = sms.Amount
		}
		sms.Currency = cmp.Or(sms.Currency, sms.ToCurrency)
		sms.ToCurrency = cmp.Or(sms.ToCurrency, sms.Currency)
	}

	if sms.Amount == 0 {
		return nil, errors.New("no amount")
	}
	return sms, nil
}
