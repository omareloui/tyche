package models

import (
	"time"

	"github.com/omareloui/financial-planner/internal/db"
)

type Item struct {
	ID       string  `validate:"required"`
	Name     string  `validate:"required" conform:"trim,title"`
	Amount   float64 `validate:"required"`
	BoughtOn time.Time
	Currency Currency
}

func (it *Item) ViewAmount() string {
	return formatAmount(it.Amount, it.Currency.Enum)
}

func (it *Item) ViewEgpAmount() string {
	return formatAmount(it.Currency.EgpFxRate*it.Amount, CurrencyEgp)
}

func DBItemRowToItem(dbIt *db.GetItemRow) *Item {
	return &Item{
		ID:       dbIt.ID,
		Name:     dbIt.Name,
		Amount:   dbIt.Amount,
		BoughtOn: dbIt.BoughtOn.Time,
		Currency: Currency{
			ID:        dbIt.CurrencyID,
			Enum:      CurrencyEnum(dbIt.Enum.Int64),
			EgpFxRate: dbIt.EgpFxRate.Float64,
		},
	}
}

func DBItemToItem(dbIt *db.Item) *Item {
	return &Item{
		ID:       dbIt.ID,
		Name:     dbIt.Name,
		Amount:   dbIt.Amount,
		BoughtOn: dbIt.BoughtOn.Time,
		Currency: Currency{
			ID: dbIt.CurrencyID,
		},
	}
}

func DBItemsRowToItems(dbIts []db.GetItemsRow) []Item {
	items := make([]Item, len(dbIts))
	for i, it := range dbIts {
		items[i] = Item{
			ID:       it.ID,
			Name:     it.Name,
			Amount:   it.Amount,
			BoughtOn: it.BoughtOn.Time,
			Currency: Currency{
				ID:        it.CurrencyID,
				Enum:      CurrencyEnum(it.Enum.Int64),
				EgpFxRate: it.EgpFxRate.Float64,
			},
		}
	}
	return items
}
