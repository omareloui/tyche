package models

import (
	"github.com/omareloui/financial-planner/internal/db"
)

type Currency struct {
	ID       string `validate:"required"`
	Currency string `conform:"trim,upper"`
	Symbol   string `conform:"trim,upper"`
}

func DBCurrencyToCurrency(dbCurrency *db.Currency) *Currency {
	return &Currency{
		ID:       dbCurrency.ID,
		Currency: dbCurrency.Currency,
		Symbol:   dbCurrency.Symbol,
	}
}

func DBCurrenciesToCurrencies(dbCurrency []db.Currency) []Currency {
	currencies := make([]Currency, len(dbCurrency))
	for i, cur := range dbCurrency {
		currencies[i] = *DBCurrencyToCurrency(&cur)
	}
	return currencies
}
