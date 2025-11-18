package models

import (
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/formatters"
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
	return formatters.FormatAmount(gl.Price, gl.Currency.Symbol)
}

func (gl *Goal) Progress() float64 {
	var sum float64

	for _, tx := range gl.Transactions {
		if !tx.FulfilledAt.IsZero() {
			sum += tx.Amount * tx.Account.Currency.FxRate(gl.Currency.String)
		}
	}

	return sum
}

func (gl *Goal) ProgressPercentage() float64 {
	return gl.Progress() / gl.Price * 100
}

func (gl *Goal) Remaining() float64 {
	return gl.Price - gl.Progress()
}

func (gl *Goal) ViewRemaining() string {
	return formatters.FormatAmount(gl.Remaining(), gl.Currency.Symbol)
}

func (gl *Goal) ViewProgress() string {
	return formatters.FormatAmount(gl.Progress(), gl.Currency.Symbol)
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
