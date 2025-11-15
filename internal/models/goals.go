package models

import (
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/numsutils"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
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

type createGoal struct {
	ID         string  `validate:"required"`
	Name       string  `validate:"required" conform:"trim,title"`
	Price      float64 `validate:"required,gt=0"`
	CurrencyID string  `validate:"required"`
}

func (gl *Goal) ViewAmount() string {
	return FormatAmount(gl.Price, gl.Currency.Symbol)
}

func (gl *Goal) Remaining() float64 {
	var sum float64

	for _, tx := range gl.Transactions {
		if !tx.FulfilledAt.IsZero() {
			sum += tx.Amount * tx.Account.Currency.FxRate(gl.Currency.String)
		}
	}
	rem := gl.Price - sum
	if rem < 0 {
		return 0
	}
	return rem
}

func (gl *Goal) ViewRemaining() string {
	return FormatAmount(gl.Remaining(), gl.Currency.Symbol)
}

func (gl *Goal) Plan(numberOfMonths int) float64 {
	return numsutils.Round(gl.Remaining() / float64(numberOfMonths))
}

func (gl *Goal) toValidate() *createGoal {
	return &createGoal{
		ID:         gl.ID,
		Name:       gl.Name,
		Price:      gl.Price,
		CurrencyID: gl.Currency.ID,
	}
}

func (gl *Goal) SanitizeAndValidate(sani *sanitizer.Sanitizer, valid *valid.Validator) error {
	glToValidate := gl.toValidate()
	if err := sani.SanitizeStruct(glToValidate); err != nil {
		return errs.ErrSanitizer
	}
	if valerr := valid.Validate(glToValidate); valerr != nil {
		return *valerr
	}
	return nil
}
