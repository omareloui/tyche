package models

import (
	"errors"
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/formatters"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

var ErrDuplicateAccountName = errors.New("duplicate account name")

type Account struct {
	ID           string
	Name         string
	Amount       float64
	ActualAmount float64
	Currency     Currency
	Kind         AccountKind
	Counterparty *Counterparty
	Iban         string
	// SmsRefs are how bank sms refer to it (account number, card, ...).
	SmsRefs            []string
	CardNumber         string
	CardLimit          float64
	NameOnCard         string
	Note               string
	IsArchive          bool
	ExcludeFromBalance bool
	IsDefault          bool
	ExpirationDate     time.Time
	CreatedAt          time.Time
	// Gamyaa is set on gam'yaa accounts.
	Gamyaa *Gamyaa
}

type createAccount struct {
	ID         string `validate:"required"`
	Name       string `validate:"required"`
	KindID     string `validate:"required,ulid"`
	Kind       uint8  `validate:"required,oneof=1 2 3 4 5"`
	Iban       string `validate:"excluded_unless=Kind 2,omitempty,iban"`
	CardNumber string `validate:"omitempty,credit_card"`
	// NOTE: add this validation when this gets merged: https://github.com/go-playground/validator/pull/1307
	// CardNumber     string `validate:"excluded_unless=Kind 2 Kind 1,omitempty,credit_card"`
	CounterpartyID string `validate:"required_if=Kind 4"`
}

func (a *Account) ViewAmount() string {
	return formatters.FormatAmount(a.Amount, a.Currency.Symbol)
}

// ViewAmountIn is the balance converted to cur, "" when there's no rate.
func (a *Account) ViewAmountIn(cur *Currency) string {
	rate := a.Currency.FxRate(cur.String)
	if rate == 0 {
		return ""
	}
	return formatters.FormatAmount(rate*a.Amount, cur.Symbol)
}

func (a *Account) ViewActualAmount() string {
	return formatters.FormatAmount(a.ActualAmount, a.Currency.Symbol)
}

func (a *Account) ViewCardLimit() string {
	return formatters.FormatAmount(a.CardLimit, a.Currency.Symbol)
}

// ViewAvailableCredit is a credit card's limit left, its balance being
// negative when spending.
func (a *Account) ViewAvailableCredit() string {
	return formatters.FormatAmount(a.CardLimit+a.Amount, a.Currency.Symbol)
}

func (a *Account) MaskIban() string {
	rs := []rune(a.Iban)
	for i := range len(rs) - 4 {
		rs[i] = '*'
	}
	return string(rs)
}

func (a *Account) MaskCardNumber() string {
	rs := []rune(a.CardNumber)
	for i := range len(rs) - 4 {
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
		Kind:           uint8(a.Kind.Enum),
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
	return formatters.FormatAmount(b.Amount, b.Currency.Symbol)
}

func (b *Balance) ViewAmountWithCurrency(cur *Currency) string {
	return formatters.FormatAmount(b.Currency.FxRate(cur.String)*b.Amount, cur.Symbol)
}

func (b *Balance) AmountWithCurrency(cur *Currency) float64 {
	return b.Currency.FxRate(cur.String) * b.Amount
}

// BalanceFix is an account's real balance, e.g. as its bank shows it.
type BalanceFix struct {
	AccountID string
	Amount    float64
}

// BalanceFixDescription describes the adjustment transactions fixing a
// balance.
const BalanceFixDescription = "Balance correction"
