package models

import (
	"github.com/omareloui/financial-plans/internal/db"
)

type Currency struct {
	ID       string
	Currency string
}

func DBCurrencyToCurrency(dbCurrency *db.Currency) *Currency {
	return &Currency{
		ID:       dbCurrency.ID,
		Currency: dbCurrency.Currency,
	}
}

func DBCurrenciesToCurrencies(dbCurrency []db.Currency) []Currency {
	currencies := make([]Currency, len(dbCurrency))
	for i, cur := range dbCurrency {
		currencies[i] = *DBCurrencyToCurrency(&cur)
	}
	return currencies
}
