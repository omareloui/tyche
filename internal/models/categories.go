package models

import (
	"math/rand"
	"time"

	"github.com/omareloui/tyche/internal/formatters"
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
	Kinds          []TxKind  `validate:"required,min=1,dive,required"`
	CreatedAt      time.Time
}

type createCategory struct {
	ID               string   `validate:"required"`
	Name             string   `validate:"required" conform:"trim,title"`
	Color            string   `validate:"required"`
	BudgetAmount     float64  `validate:"required_with=BudgetCurrency"`
	BudgetCurrencyID string   `validate:"required_with=BudgetAmount"`
	Kinds            []TxKind `validate:"required,min=1,dive,required"`
}

func (cat *Category) ViewBudgetAmount() string {
	return formatters.FormatAmount(cat.BudgetAmount, cat.BudgetCurrency.String)
}

func (cat *Category) toValidate() *createCategory {
	var currID string
	if cat.BudgetCurrency != nil {
		currID = cat.BudgetCurrency.ID
	}

	createCat := createCategory{
		ID:               cat.ID,
		Name:             cat.Name,
		Color:            cat.Color,
		BudgetAmount:     cat.BudgetAmount,
		Kinds:            cat.Kinds,
		BudgetCurrencyID: currID,
	}
	return &createCat
}

func (cat *Category) Validate(valid *valid.Validator) error {
	catToValidate := cat.toValidate()
	if valerr := valid.Validate(catToValidate); valerr != nil {
		return *valerr
	}
	return nil
}
