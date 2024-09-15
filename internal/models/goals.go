package models

import (
	"slices"
	"time"

	"github.com/omareloui/financial-planner/internal/db"
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

// TEST: does the LIMIT work here?
func DBGoalRowToGoal(dbGl *db.GetGoalRow) *Goal {
	// TODO: popuate the items
	items := []Item{}

	return &Goal{
		ID:          dbGl.ID,
		Title:       dbGl.Title,
		EgpSubtotal: dbGl.EgpSubtotal,
		CreatedAt:   dbGl.CreatedAt.(time.Time),
		Items:       items,
	}
}

func DBGoalRowsToGoals(dbGls []db.GetGoalsRow) []Goal {
	gls := []Goal{}

	for _, dbGl := range dbGls {
		glIdx := slices.IndexFunc(gls, func(gl Goal) bool { return gl.ID == dbGl.ID })
		if glIdx == -1 {
			gls = append(gls, Goal{
				ID:          dbGl.ID,
				Title:       dbGl.Title,
				EgpSubtotal: dbGl.EgpSubtotal,
				CreatedAt:   dbGl.CreatedAt.(time.Time),
				Items:       []Item{},
			})
			glIdx = len(gls) - 1
		}

		gls[glIdx].Items = append(gls[glIdx].Items, Item{
			ID:          dbGl.ItemID.String,
			Name:        dbGl.ItemName.String,
			Price:       dbGl.ItemPrice.Float64,
			PurchasedOn: dbGl.ItemPurchasedOn.(time.Time),
			Currency: Currency{
				ID:        dbGl.ItemCurrencyID.String,
				Enum:      CurrencyEnum(dbGl.ItemCurrency.Int64),
				EgpFxRate: dbGl.ItemCurrencyFxRate.Float64,
			},
		})
	}

	return gls
}
