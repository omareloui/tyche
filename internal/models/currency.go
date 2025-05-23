package models

import (
	"github.com/omareloui/tyche/internal/fxrate"
)

type CurrencyEnum uint8

const (
	CurrencyNil CurrencyEnum = iota
	CurrencyUsd
	CurrencyEgp
	CurrencyAed
)

func GetDefaultCurrency() CurrencyEnum {
	// TODO: get the currency according to the ip addr of the user
	return CurrencyUsd
}

func (cs CurrencyEnum) String() string {
	return [...]string{
		"???", "USD",
		"EGP", "AED",
	}[cs]
}

func (cs CurrencyEnum) Symbol() string {
	return [...]string{
		"???", "US$",
		"EG£", "AED",
	}[cs]
}

func (cs CurrencyEnum) IbanPrefix() string {
	return [...]string{
		"??", "US",
		"EG", "AE",
	}[cs]
}

type Currency struct {
	ID   string `validate:"required"`
	Enum CurrencyEnum
}

func (c *Currency) FxRate(to CurrencyEnum) float64 {
	v, _ := fxrate.GetFxRate(c.Enum.String(), to.String())
	return v
}
