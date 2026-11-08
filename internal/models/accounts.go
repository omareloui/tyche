package models

import (
	"errors"
	"maps"
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

var ErrDuplicateAccountName = errors.New("duplicate account name")

type Account struct {
	ID           string
	Name         string
	Amount       float64
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
	// DeletedAt is set on deleted accounts, kept for their transactions'
	// history.
	DeletedAt time.Time
	// SetAside is the money moved from it to goals in progress, out of its
	// balance though still in the bank.
	SetAside float64
	// Gamyaa is set on gam'yaa accounts.
	Gamyaa *Gamyaa
	// Cycle is a credit card's statement cycle.
	Cycle CardCycle
	// Statement is a credit card's last statement, set where it's shown.
	Statement *CardStatement
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

// AmountUnit is the unit an amount of a, a metal account, is typed in: the
// picked one, else the one its currency is shown in; "" for other accounts.
func (a *Account) AmountUnit(picked string) MetalUnit {
	if a == nil || !a.Currency.IsMetal() {
		return ""
	}
	if u := ParseMetalUnit(picked); u != "" {
		return u
	}
	return a.Currency.Unit()
}

func (a *Account) ViewAmount() string {
	return a.Currency.Format(a.Amount)
}

// ViewAmountIn is the balance converted to cur, "" when there's no rate.
func (a *Account) ViewAmountIn(cur *Currency) string {
	rate := a.Currency.FxRate(cur.String)
	if rate == 0 {
		return ""
	}
	return cur.Format(rate * a.Amount)
}

func (a *Account) ViewSetAside() string {
	return a.Currency.Format(a.SetAside)
}

// InBank is what the bank shows: its balance plus what's set aside for goals.
func (a *Account) InBank() float64 { return a.Amount + a.SetAside }

func (a *Account) ViewInBank() string {
	return a.Currency.Format(a.InBank())
}

func (a *Account) IsDeleted() bool { return !a.DeletedAt.IsZero() }

// DisplayName is its name, marked when it's deleted.
func (a *Account) DisplayName() string {
	if a.IsDeleted() {
		return a.Name + " (deleted)"
	}
	return a.Name
}

// IsGoal reports whether it's a goal's account.
func (a *Account) IsGoal() bool { return a.Kind.Enum == AccKindGoal }

func (a *Account) ViewCardLimit() string {
	return a.Currency.Format(a.CardLimit)
}

// ViewAvailableCredit is a credit card's limit left, its balance being
// negative when spending.
func (a *Account) ViewAvailableCredit() string {
	return a.Currency.Format(a.CardLimit + a.Amount)
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
	valerr := valid.Validate(ac)
	if a.Kind.Enum == AccKindCreditCard {
		if ce := a.Cycle.Validate(); len(ce) > 0 {
			if valerr == nil {
				valerr = &errs.ValidationError{Errors: errs.ValidationErrors{}}
			}
			maps.Copy(valerr.Errors, ce)
		}
	}
	if valerr != nil {
		return *valerr
	}
	return nil
}

type Balance struct {
	Amount   float64 `validate:"require"`
	Currency Currency
}

func (b *Balance) ViewAmount() string {
	return b.Currency.Format(b.Amount)
}

func (b *Balance) ViewAmountWithCurrency(cur *Currency) string {
	return cur.Format(b.Currency.FxRate(cur.String) * b.Amount)
}

func (b *Balance) AmountWithCurrency(cur *Currency) float64 {
	return b.Currency.FxRate(cur.String) * b.Amount
}

// BalanceFix is an account's real balance, e.g. as its bank shows it.
type BalanceFix struct {
	AccountID string
	Amount    float64
}

// BankEntry is what a's balance is typed as when fixing it: what the bank
// shows (InBank, so what's set aside for goals included), signed like the
// balance (a credit card's debt negative).
func (a *Account) BankEntry() float64 {
	return a.InBank()
}

// FixOfBankEntry is the BalanceFix of a typed BankEntry (in the stored unit).
func (a *Account) FixOfBankEntry(entry float64) BalanceFix {
	return BalanceFix{AccountID: a.ID, Amount: entry - a.SetAside}
}

// BalanceFixDescription describes the adjustment transactions fixing a
// balance.
const BalanceFixDescription = "Balance correction"
