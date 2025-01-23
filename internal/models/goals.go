package models

import (
	"time"

	"github.com/omareloui/tyche/internal/numsutils"
)

type Goal struct {
	ID           string  `validate:"required"`
	Name         string  `validate:"required" conform:"trim,title"`
	Price        float64 `validate:"required,gt=0"`
	Currency     Currency
	FulfilledAt  time.Time
	CreatedAt    time.Time
	Transactions []Transaction
}

func (gl *Goal) ViewAmount() string {
	return formatAmount(gl.Price, gl.Currency.Enum)
}

func (gl *Goal) Remaining() float64 {
	sum := float64(0)
	for _, tx := range gl.Transactions {
		if !tx.FulfilledAt.IsZero() {
			sum += tx.Amount * tx.Account.Currency.FxRate(gl.Currency.Enum)
		}
	}
	rem := gl.Price - sum
	if rem < 0 {
		return 0
	}
	return rem
}

func (gl *Goal) ViewRemaining() string {
	return formatAmount(gl.Remaining(), gl.Currency.Enum)
}

func (gl *Goal) Plan(numberOfMonths int) float64 {
	return numsutils.Round(gl.Remaining() / float64(numberOfMonths))
}
