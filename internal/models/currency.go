package models

import (
	"github.com/omareloui/tyche/internal/fxrate"
)

type CurrencyKind string

const (
	CurrencyKindNil           CurrencyKind = "???"
	CurrencyKindFiat          CurrencyKind = "Fiat"
	CurrencyKindCrypto        CurrencyKind = "Crypto"
	CurrencyKindPreciousMetal CurrencyKind = "PreciousMetal"
)

func (ck CurrencyKind) String() string {
	if ck == CurrencyKindPreciousMetal {
		return "Precious Metal"
	}
	return string(ck)
}

type Currency struct {
	ID         string       `validate:"required"`
	String     string       `validate:"required,max=5"`
	Symbol     string       `validate:"required,max=5,alphanum"`
	IbanPrefix string       `validate:"len=2,alpha"`
	Kind       CurrencyKind `validate:"required"`
	IsShown    bool
	IsDefault  bool
}

func (c *Currency) FxRate(toCurrency string) float64 {
	v, _ := fxrate.GetFxRate(c.String, toCurrency)
	return v
}

func GetDefaultCurrency() *Currency {
	// TODO: get the currency according to the ip addr of the user
	return &Currency{
		String:     "USD",
		Symbol:     "US$",
		IbanPrefix: "US",
	}
}
