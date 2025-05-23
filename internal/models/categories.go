package models

import (
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

type Category struct {
	ID             string
	Name           string    `validate:"required" conform:"trim,title"`
	BudgetAmount   float64   `validate:"required_with=BudgetCurrency"`
	BudgetCurrency *Currency `validate:"required_with=BudgetAmount"`
	Kind           TransactionKind
	CreatedAt      time.Time
}

type createCategory struct {
	ID               string  `validate:"required"`
	Name             string  `validate:"required" conform:"trim,title"`
	BudgetAmount     float64 `validate:"required_with=BudgetCurrency"`
	BudgetCurrencyID string  `validate:"required_with=BudgetAmount"`
	KindID           string  `validate:"required"`
}

func (cat *Category) ViewBudgetAmount() string {
	var cur string
	if cat.BudgetCurrency == nil {
		cur = GetDefaultCurrency().String
	} else {
		cur = cat.BudgetCurrency.String
	}
	return formatAmount(cat.BudgetAmount, cur)
}

func (cat *Category) toValidate() *createCategory {
	var currId string
	if cat.BudgetCurrency != nil {
		currId = cat.BudgetCurrency.ID
	}

	return &createCategory{
		ID:               cat.ID,
		Name:             cat.Name,
		BudgetAmount:     cat.BudgetAmount,
		KindID:           cat.Kind.ID,
		BudgetCurrencyID: currId,
	}
}

func (cat *Category) SanitizeAndValidate(sani *sanitizer.Sanitizer, valid *valid.Validator) error {
	catToValidate := cat.toValidate()
	if err := sani.SanitizeStruct(catToValidate); err != nil {
		return errs.ErrSanitizer
	}
	if valerr := valid.Validate(catToValidate); valerr != nil {
		return *valerr
	}
	return nil
}
