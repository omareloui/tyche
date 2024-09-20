package models

import (
	"time"
)

type Item struct {
	ID          string  `validate:"required"`
	Name        string  `validate:"required" conform:"trim,title"`
	Price       float64 `validate:"required"`
	PurchasedOn time.Time
	Currency    Currency
}

func (it *Item) EgpPrice() float64 {
	return it.Currency.EgpFxRate * it.Price
}

func (it *Item) ViewPrice() string {
	return formatAmount(it.Price, it.Currency.Enum)
}

func (it *Item) ViewEgpPrice() string {
	return formatAmount(it.EgpPrice(), CurrencyEgp)
}
