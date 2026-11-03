package models

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/omareloui/tyche/internal/formatters"
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
	// InGrams shows a precious metal's amounts in grams instead of troy
	// ounces, the unit its rates and stored amounts are in.
	InGrams bool
}

// GramsPerTroyOunce is the weight of a troy ounce, the unit precious metals
// are priced in.
const GramsPerTroyOunce = 31.1034768

// MetalUnit is a unit precious metal amounts are shown or entered in.
type MetalUnit string

const (
	MetalUnitOunce MetalUnit = "oz"
	MetalUnitGram  MetalUnit = "g"
)

// metalCodes are the precious metals' codes (and symbols), as the currencies
// joined to a row often carry only those.
var metalCodes = []string{"XAU", "XAG", "XPT", "XPD"}

func (c *Currency) IsMetal() bool {
	return c.Kind == CurrencyKindPreciousMetal ||
		slices.Contains(metalCodes, strings.ToUpper(c.String)) ||
		slices.Contains(metalCodes, strings.ToUpper(c.Symbol))
}

// Unit is the unit c's amounts are shown in, "" for anything but a metal.
func (c *Currency) Unit() MetalUnit {
	if !c.IsMetal() {
		return ""
	}
	if c.InGrams {
		return MetalUnitGram
	}
	return MetalUnitOunce
}

// ToUnit converts a stored amount (troy ounces for metals) to u.
func ToUnit(amount float64, u MetalUnit) float64 {
	if u == MetalUnitGram {
		return amount * GramsPerTroyOunce
	}
	return amount
}

// FromUnit converts an amount in u back to the stored unit.
func FromUnit(amount float64, u MetalUnit) float64 {
	if u == MetalUnitGram {
		return amount / GramsPerTroyOunce
	}
	return amount
}

// MetalUnits are the units a metal's amounts can be shown or entered in.
var MetalUnits = []MetalUnit{MetalUnitOunce, MetalUnitGram}

// ParseMetalUnit reads a unit, "" when it's none.
func ParseMetalUnit(s string) MetalUnit {
	switch u := MetalUnit(s); u {
	case MetalUnitOunce, MetalUnitGram:
		return u
	}
	return ""
}

// AmountDigits is how many fraction digits amounts in u keep, shown or
// entered.
func AmountDigits(u MetalUnit) int {
	if u == MetalUnitOunce {
		return 4
	}
	return 2
}

// Shown is a stored amount in the unit c is shown in.
func (c *Currency) Shown(amount float64) float64 {
	return ToUnit(amount, c.Unit())
}

// Format formats an amount of c, a metal's in its unit (e.g. "XAU 12.5 g").
func (c *Currency) Format(amount float64) string {
	return c.FormatIn(amount, c.Unit())
}

// FormatIn is Format in the unit u, for metals.
func (c *Currency) FormatIn(amount float64, u MetalUnit) string {
	if !c.IsMetal() {
		return formatters.FormatAmount(amount, c.Symbol)
	}
	return formatters.FormatAmountDigits(ToUnit(amount, u), c.Symbol, AmountDigits(u)) + " " + string(u)
}

// Round rounds a stored amount of c to its precision: a metal's troy ounces
// keep enough digits for a gram's hundredths.
func (c *Currency) Round(amount float64) float64 {
	pow := 100.0
	if c.IsMetal() {
		pow = 1e6
	}
	return math.Round(amount*pow) / pow
}

// InputValue is a stored amount as typed in an amount input in the unit u,
// "" for 0.
func InputValue(amount float64, u MetalUnit) string {
	if amount == 0 {
		return ""
	}
	return strconv.FormatFloat(ToUnit(amount, u), 'f', AmountDigits(u), 64)
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
