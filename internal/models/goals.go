package models

import (
	"time"
)

type Goal struct {
	ID          string  `validate:"required"`
	Title       string  `validate:"required" conform:"trim,title"`
	EgpSubtotal float64 `validate:"required,gt=0"`
	CreatedAt   time.Time

	Items []Item `validate:"required,min=1"`
}

func (gl *Goal) ViewAmount() string {
	return formatAmount(gl.EgpSubtotal, CurrencyEgp)
}

func (gl *Goal) CalculateEgpSubtotal() float64 {
	sum := float64(0)
	for _, it := range gl.Items {
		sum += it.EgpPrice()
	}
	return sum
}
