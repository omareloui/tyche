package models

import (
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

// SMSTemplateAction is what's done with an sms matching a template.
type SMSTemplateAction string

const (
	// SMSTemplateTransaction creates a transaction from the sms.
	SMSTemplateTransaction SMSTemplateAction = "transaction"
	// SMSTemplateIgnore marks the sms as ignored, e.g. statements, OTPs or
	// declined payments.
	SMSTemplateIgnore SMSTemplateAction = "ignore"
)

// SMSTemplateScope is where a template is stored.
type SMSTemplateScope string

const (
	// SMSTemplateShared templates live in the shared database, used for
	// everyone.
	SMSTemplateShared SMSTemplateScope = "shared"
	// SMSTemplateMine templates live in the user's database, tried before the
	// shared ones.
	SMSTemplateMine SMSTemplateScope = "mine"
)

func ParseSMSTemplateScope(s string) (SMSTemplateScope, bool) {
	switch sc := SMSTemplateScope(s); sc {
	case SMSTemplateShared, SMSTemplateMine:
		return sc, true
	}
	return "", false
}

// SMSTemplateFixedFields are the fields a template can set to a constant
// instead of reading them from the sms, e.g. "Cash" as the account receiving
// an ATM withdrawal.
var SMSTemplateFixedFields = []string{"account_id", "to_account_id", "counterparty", "note"}

// SMSTemplate describes the text of a bank sms, see smsparser for its syntax.
type SMSTemplate struct {
	ID    string
	Scope SMSTemplateScope
	Name  string
	// Bank is a label, it's not used for matching.
	Bank string
	// Senders limit the template to sms from these senders (case-insensitive),
	// empty for any sender.
	Senders []string
	Action  SMSTemplateAction
	TxKind  TxKind
	Body    string
	// Fixed are constant values of fields, see SMSTemplateFixedFields.
	Fixed map[string]string
	// Sample is an sms the template has to match.
	Sample string
	// Priority orders the templates, higher ones are tried first.
	Priority  int
	IsEnabled bool
	// SeedKey identifies the shared templates seeded from the code.
	SeedKey   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type createSMSTemplate struct {
	ID     string `validate:"required"`
	Name   string `validate:"required" conform:"trim"`
	Bank   string `conform:"trim"`
	Action string `validate:"oneof=transaction ignore"`
	TxKind TxKind `validate:"required_if=Action transaction,omitempty,oneof=1 2 3 5"`
	Body   string `validate:"required,not_blank"`
	Sample string
}

func (t *SMSTemplate) SanitizeAndValidate(sani *sanitizer.Sanitizer, valid *valid.Validator) error {
	t.Body = strings.TrimSpace(t.Body)
	t.Sample = strings.TrimSpace(t.Sample)

	senders := t.Senders[:0]
	for _, s := range t.Senders {
		if s = strings.TrimSpace(s); s != "" {
			senders = append(senders, s)
		}
	}
	t.Senders = senders

	for k, v := range t.Fixed {
		if v = strings.TrimSpace(v); v == "" {
			delete(t.Fixed, k)
		} else {
			t.Fixed[k] = v
		}
	}
	if t.Action == SMSTemplateIgnore {
		t.TxKind, t.Fixed = NilTxKind, nil
	}

	v := &createSMSTemplate{
		ID:     t.ID,
		Name:   t.Name,
		Bank:   t.Bank,
		Action: string(t.Action),
		TxKind: t.TxKind,
		Body:   t.Body,
		Sample: t.Sample,
	}
	if err := sani.SanitizeStruct(v); err != nil {
		return errs.ErrSanitizer
	}
	t.Name, t.Bank = v.Name, v.Bank

	if valerr := valid.Validate(v); valerr != nil {
		return *valerr
	}
	return nil
}

// AppliesTo is whether the template can be used for an sms from the sender.
func (t *SMSTemplate) AppliesTo(sender string) bool {
	sender = strings.TrimSpace(sender)
	for _, s := range t.Senders {
		if strings.EqualFold(strings.TrimSpace(s), sender) {
			return true
		}
	}
	return false
}
