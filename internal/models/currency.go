package models

import (
	"github.com/omareloui/financial-planner/internal/db"
)

type CurrencyEnum uint8

const (
	CurrencyEgp CurrencyEnum = iota
	CurrencyUsd
)

const DfltCurrency = CurrencyEgp

func (cs CurrencyEnum) String() string {
	return [...]string{"EGP", "USD"}[cs]
}

func (cs CurrencyEnum) Symbol() string {
	return [...]string{"EG£", "US$"}[cs]
}

type Currency struct {
	ID        string `validate:"required"`
	Enum      CurrencyEnum
	EgpFxRate float64
}

func DBCurrencyToCurrency(dbCurr *db.Currency) *Currency {
	return &Currency{
		ID:        dbCurr.ID,
		Enum:      CurrencyEnum(dbCurr.Enum),
		EgpFxRate: dbCurr.EgpFxRate,
	}
}

func DBCurrenciesToCurrencies(dbCurrency []db.Currency) []Currency {
	currencies := make([]Currency, len(dbCurrency))
	for i, cur := range dbCurrency {
		currencies[i] = *DBCurrencyToCurrency(&cur)
	}
	return currencies
}
