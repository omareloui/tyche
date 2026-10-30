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

// ValueIn is what one unit of c is worth in base. It reads base's rate table,
// so valuing a whole list costs one fetch instead of one per currency.
func (c *Currency) ValueIn(base string) float64 {
	v, _ := fxrate.GetFxRate(base, c.String)
	if v == 0 {
		return 0
	}
	return 1 / v
}

func GetDefaultCurrency() *Currency {
	// TODO: get the currency according to the ip addr of the user
	return &Currency{
		String:     "USD",
		Symbol:     "US$",
		IbanPrefix: "US",
	}
}
