package models

import (
	"time"

	"github.com/omareloui/financial-planner/internal/db"
)

type Item struct {
	ID          string  `validate:"required"`
	Name        string  `validate:"required" conform:"trim,title"`
	Price       float64 `validate:"required"`
	PurchasedOn time.Time
	Currency    Currency
}

func (it *Item) EgpPrice() float64 {
	return it.Currency.EgpFxRate * it.Price
}

func (it *Item) ViewPrice() string {
	return formatAmount(it.Price, it.Currency.Enum)
}

func (it *Item) ViewEgpPrice() string {
	return formatAmount(it.EgpPrice(), CurrencyEgp)
}

func DBItemRowToItem(dbIt *db.GetItemRow) *Item {
	return &Item{
		ID:          dbIt.ID,
		Name:        dbIt.Name,
		Price:       dbIt.Price,
		PurchasedOn: dbIt.PurchasedOn.(time.Time),
		Currency: Currency{
			ID:        dbIt.CurrencyID,
			Enum:      CurrencyEnum(dbIt.Enum.Int64),
			EgpFxRate: dbIt.EgpFxRate.Float64,
		},
	}
}

// func DBItemToItem(dbIt *db.Item) *Item {
// 	return &Item{
// 		ID:          dbIt.ID,
// 		Name:        dbIt.Name,
// 		Price:       dbIt.Amount,
// 		PurchasedOn: dbIt.BoughtOn.Time,
// 		Currency: Currency{
// 			ID: dbIt.CurrencyID,
// 		},
// 	}
// }

func DBItemsRowToItems(dbIts []db.GetItemsRow) []Item {
	items := make([]Item, len(dbIts))
	for i, it := range dbIts {
		items[i] = Item{
			ID:          it.ID,
			Name:        it.Name,
			Price:       it.Price,
			PurchasedOn: it.PurchasedOn.(time.Time),
			Currency: Currency{
				ID:        it.CurrencyID,
				Enum:      CurrencyEnum(it.Enum.Int64),
				EgpFxRate: it.EgpFxRate.Float64,
			},
		}
	}
	return items
}
