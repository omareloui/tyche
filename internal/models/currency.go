package models

import (
	"github.com/omareloui/tyche/internal/fxrate"
)

type CurrencyEnum uint8

const (
	CurrencyNil CurrencyEnum = iota
	CurrencyEgp
	CurrencyUsd
	CurrencyAed
)

const DfltCurrency = CurrencyEgp

func (cs CurrencyEnum) String() string {
	return [...]string{
		"???", "EGP",
		"USD", "AED",
	}[cs]
}

func (cs CurrencyEnum) Symbol() string {
	return [...]string{
		"???", "EG£",
		"US$", "AED",
	}[cs]
}

func (cs CurrencyEnum) IbanPrefix() string {
	return [...]string{
		"??", "EG",
		"US", "AE",
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
