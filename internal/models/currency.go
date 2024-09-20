package models

type CurrencyEnum uint8

const (
	CurrencyEgp CurrencyEnum = iota + 1
	CurrencyUsd
)

const DfltCurrency = CurrencyEgp

func (cs CurrencyEnum) String() string {
	return [...]string{"EGP", "USD"}[cs-1]
}

func (cs CurrencyEnum) Symbol() string {
	return [...]string{"EG£", "US$"}[cs-1]
}

type Currency struct {
	ID        string `validate:"required"`
	Enum      CurrencyEnum
	EgpFxRate float64
}
