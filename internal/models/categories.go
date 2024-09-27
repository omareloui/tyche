package models

import (
	"time"
)

type Category struct {
	ID          string `validate:"required"`
	Name        string `validate:"required" conform:"trim,title"`
	BudgetInEgp float64
	CreatedAt   time.Time
}

func (it *Category) ViewBudgetInEgp() string {
	return formatAmount(it.BudgetInEgp, CurrencyEgp)
}
