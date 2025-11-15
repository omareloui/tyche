package models

import (
	"math/rand"
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

var CategoryColors = []string{
	"red", "orange", "amber", "yellow", "lime", "green",
	"emerald", "teal", "cyan", "sky", "blue", "indigo",
	"violet", "purple", "fuchsia", "pink", "rose", "gray",
}

func GetRandomCategoryColor() string {
	return CategoryColors[rand.Intn(len(CategoryColors))]
}

func CategoryColorOptions() map[string]string {
	options := make(map[string]string, len(CategoryColors))
	for _, color := range CategoryColors {
		options[color] = color
	}
	return options
}

func CategoryColorToHex(color string) string {
	colorMap := map[string]string{
		"red":     "#ef4444",
		"orange":  "#f97316",
		"amber":   "#f59e0b",
		"yellow":  "#eab308",
		"lime":    "#84cc16",
		"green":   "#22c55e",
		"emerald": "#10b981",
		"teal":    "#14b8a6",
		"cyan":    "#06b6d4",
		"sky":     "#0ea5e9",
		"blue":    "#3b82f6",
		"indigo":  "#6366f1",
		"violet":  "#8b5cf6",
		"purple":  "#a855f7",
		"fuchsia": "#d946ef",
		"pink":    "#ec4899",
		"rose":    "#f43f5e",
		"gray":    "#6b7280",
	}
	if hex, ok := colorMap[color]; ok {
		return hex
	}
	return colorMap["gray"]
}

type Category struct {
	ID             string
	Name           string    `validate:"required" conform:"trim,title"`
	Color          string    `validate:"required"`
	BudgetAmount   float64   `validate:"required_with=BudgetCurrency"`
	BudgetCurrency *Currency `validate:"required_with=BudgetAmount"`
	Kind           TransactionKind
	CreatedAt      time.Time
}

type createCategory struct {
	ID               string  `validate:"required"`
	Name             string  `validate:"required" conform:"trim,title"`
	Color            string  `validate:"required"`
	BudgetAmount     float64 `validate:"required_with=BudgetCurrency"`
	BudgetCurrencyID string  `validate:"required_with=BudgetAmount"`
	KindID           string  `validate:"required"`
}

func (cat *Category) ViewBudgetAmount() string {
	return FormatAmount(cat.BudgetAmount, cat.BudgetCurrency.String)
}

func (cat *Category) toValidate() *createCategory {
	var currId string
	if cat.BudgetCurrency != nil {
		currId = cat.BudgetCurrency.ID
	}

	return &createCategory{
		ID:               cat.ID,
		Name:             cat.Name,
		Color:            cat.Color,
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
