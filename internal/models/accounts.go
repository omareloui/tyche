package models

import (
	"errors"
	"time"
)

var ErrDuplicateAccountName = errors.New("duplicate account name")

type Account struct {
	ID                 string `validate:"required"`
	Name               string `validate:"required" conform:"trim,title"`
	Amount             float64
	Currency           Currency
	Kind               AccountKind
	IBAN               string `validate:"omitempty,iban" conform:"trim,nospaces"`
	CardNumber         string `validate:"omitempty" conform:"trim,nospaces"`
	Note               string `conform:"trim"`
	IsArchive          bool
	ExcludeFromBalance bool
	CreatedAt          time.Time
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
