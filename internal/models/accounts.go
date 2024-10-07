package models

import (
	"errors"
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

var ErrDuplicateAccountName = errors.New("duplicate account name")

type Account struct {
	ID                 string
	Name               string
	Amount             float64
	ActualAmount       float64
	Currency           Currency
	Kind               AccountKind
	Counterparty       *Counterparty
	Iban               string
	CardNumber         string
	Note               string
	IsArchive          bool
	ExcludeFromBalance bool
	IsDefault          bool
	CreatedAt          time.Time
}

type createAccount struct {
	ID         string `validate:"required"`
	Name       string `validate:"required"`
	KindID     string `validate:"required,ulid"`
	KindString string `validate:"required,oneof='Credit Card' 'Bank Account' Cash Debt"`
	Iban       string `validate:"excluded_unless=KindString 'Bank Account',omitempty,iban"`
	CardNumber string `validate:"omitempty,credit_card"`
	// NOTE: add this validation when this gets merged: https://github.com/go-playground/validator/pull/1307
	// CardNumber     string `validate:"excluded_unless=KindString 'Bank Account' KindString 'Credit Card',omitempty,credit_card"`
	CounterpartyID string `validate:"required_if=KindString Debt"`
}

func (a *Account) ViewAmount() string {
	return formatAmount(a.Amount, a.Currency.Enum)
}

func (a *Account) ViewActualAmount() string {
	return formatAmount(a.ActualAmount, a.Currency.Enum)
}

func (a *Account) MaskIban() string {
	rs := []rune(a.Iban)
	for i := 0; i < len(rs)-4; i++ {
		rs[i] = '*'
	}
	return string(rs)
}

func (a *Account) MaskCardNumber() string {
	rs := []rune(a.CardNumber)
	for i := 0; i < len(rs)-4; i++ {
		rs[i] = '*'
	}
	return string(rs)
}

func (a *Account) toValidate() *createAccount {
	cpId := ""

	if a.Counterparty != nil {
		cpId = a.Counterparty.ID
	}

	return &createAccount{
		ID:             a.ID,
		Name:           a.Name,
		KindID:         a.Kind.ID,
		KindString:     a.Kind.Enum.String(),
		Iban:           a.Iban,
		CardNumber:     a.CardNumber,
		CounterpartyID: cpId,
	}
}

func (a *Account) SanitizeAndValidate(sani *sanitizer.Sanitizer, valid *valid.Validator) error {
	ac := a.toValidate()
	if err := sani.SanitizeStruct(ac); err != nil {
		return errs.ErrSanitizer
	}
	if valerr := valid.Validate(ac); valerr != nil {
		return *valerr
	}
	return nil
}

type Balance struct {
	Amount   float64 `validate:"require"`
	Currency Currency
}

func (b *Balance) ViewAmount() string {
	return formatAmount(b.Amount, b.Currency.Enum)
}

func (b *Balance) ViewEgpAmount() string {
	return formatAmount(b.Currency.EgpFxRate*b.Amount, CurrencyEgp)
}

func (b *Balance) EgpAmount() float64 {
	return b.Currency.EgpFxRate * b.Amount
}
