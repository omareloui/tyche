package models

import (
	"time"

	"github.com/omareloui/tyche/internal/numsutils"
)

type Goal struct {
	ID           string  `validate:"required"`
	Title        string  `validate:"required" conform:"trim,title"`
	EgpSubtotal  float64 `validate:"required,gt=0"`
	FulfilledAt  time.Time
	CreatedAt    time.Time
	Items        []Item `validate:"required,min=1"`
	Transactions []Transaction
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

func (gl *Goal) Remaining() float64 {
	sum := float64(0)
	for _, tx := range gl.Transactions {
		if !tx.FulfilledAt.IsZero() {
			sum += tx.Amount * tx.Account.Currency.FxRate(CurrencyEgp)
		}
	}
	rem := gl.EgpSubtotal - sum
	if rem < 0 {
		return 0
	}
	return rem
}

func (gl *Goal) ViewRemaining() string {
	return formatAmount(gl.Remaining(), CurrencyEgp)
}

func (gl *Goal) Plan(numberOfMonths int) float64 {
	return numsutils.Round(gl.Remaining() / float64(numberOfMonths))
}
