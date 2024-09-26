package models

import (
	"errors"
	"time"
)

var ErrDuplicateAccountName = errors.New("duplicate account name")

type Account struct {
	ID                 string `validate:"required"`
	Name               string `conform:"trim,title"`
	Amount             float64
	Currency           Currency
	Kind               AccountKind
	IBAN               string `validate:"omitempty,iban" conform:"trim,nospaces"`
	CardNumber         string `validate:"omitempty" conform:"trim,num"`
	Note               string `conform:"trim"`
	IsArchive          bool
	ExcludeFromBalance bool
	IsDefault          bool
	CreatedAt          time.Time
}

// This is used to validate the
// account on create and update only
type AccountCreation struct {
	ID         string `validate:"required"`
	Name       string `validate:"required"`
	IBAN       string `validate:"omitempty,iban"`
	CardNumber string `validate:"omitempty,credit_card"`
}

func (a *Account) ViewAmount() string {
	return formatAmount(a.Amount, a.Currency.Enum)
}

func (a *Account) MaskIBAN() string {
	rs := []rune(a.IBAN)
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
