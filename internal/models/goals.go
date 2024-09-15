package models

import (
	"slices"
	"time"

	"github.com/omareloui/financial-planner/internal/db"
)

type Goal struct {
	ID    string `validate:"required"`
	Title string `validate:"required" conform:"trim,title"`
	// TODO: rename to total or subtotal
	EgpSumTotal float64 `validate:"required,gt=0"`
	CreatedAt   time.Time

	Items []Item `validate:"required,min=1"`
}

func (gl *Goal) ViewAmount() string {
	return formatAmount(gl.EgpSumTotal, CurrencyEgp)
}

func (gl *Goal) CalculateEgpSubtotal() float64 {
	sum := float64(0)
	for _, it := range gl.Items {
		sum += it.EgpAmount()
	}
	return sum
}

func DBGoalToGoal(dbGl *db.Goal) *Goal {
	return &Goal{
		ID:          dbGl.ID,
		Title:       dbGl.Title,
		EgpSumTotal: dbGl.EgpSumTotal,
		CreatedAt:   dbGl.CreatedAt.(time.Time),
	}
}

func DBGoalRowToGoal(dbGl *db.GetGoalRow) *Goal {
	items := []Item{}

	return &Goal{
		ID:          dbGl.ID,
		Title:       dbGl.Title,
		EgpSumTotal: dbGl.EgpSumTotal,
		CreatedAt:   dbGl.CreatedAt.(time.Time),

		Items: items,
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
				EgpSumTotal: dbGl.EgpSumTotal,
				CreatedAt:   dbGl.CreatedAt.(time.Time),
				Items:       []Item{},
			})
			glIdx = len(gls) - 1
		}

		gls[glIdx].Items = append(gls[glIdx].Items, Item{
			ID:       dbGl.ItemID.String,
			Name:     dbGl.ItemName.String,
			Amount:   dbGl.ItemAmount.Float64,
			BoughtOn: dbGl.ItemBoughtOn.Time,
			Currency: Currency{
				ID:        dbGl.ItemCurrencyID.String,
				Enum:      CurrencyEnum(dbGl.ItemCurrency.Int64),
				EgpFxRate: dbGl.ItemCurrencyFxRate.Float64,
			},
		})
	}

	return gls
}

func DBGoalsToGoals(dbGl []db.Goal) []Goal {
	goals := make([]Goal, len(dbGl))
	for i, gl := range dbGl {
		goals[i] = *DBGoalToGoal(&gl)
	}
	return goals
}
