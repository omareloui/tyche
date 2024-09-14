package models

import (
	"github.com/omareloui/financial-planner/internal/db"
)

type Item struct {
	ID       string  `validate:"required"`
	Name     string  `validate:"required" conform:"trim,title"`
	Amount   float64 `validate:"required"`
	Bought   bool
	Currency Currency
}

func (it *Item) ViewAmount() string {
	return formatAmount(it.Amount, it.Currency)
}

func DBItemRowToItem(dbIt *db.GetItemRow) *Item {
	return &Item{
		ID:     dbIt.ID,
		Name:   dbIt.Name,
		Amount: dbIt.Amount,
		Bought: dbIt.Bought.Bool,
		Currency: Currency{
			ID:       dbIt.CurrencyID,
			Currency: dbIt.Currency.String,
			Symbol:   dbIt.Symbol.String,
		},
	}
}

func DBItemToItem(dbIt *db.Item) *Item {
	return &Item{
		ID:     dbIt.ID,
		Name:   dbIt.Name,
		Amount: dbIt.Amount,
		Bought: dbIt.Bought.Bool,
		Currency: Currency{
			ID: dbIt.CurrencyID,
		},
	}
}

func DBItemsRowToItems(dbItem []db.GetItemsRow) []Item {
	items := make([]Item, len(dbItem))
	for i, it := range dbItem {
		items[i] = Item{
			ID:     it.ID,
			Name:   it.Name,
			Amount: it.Amount,
			Bought: it.Bought.Bool,
			Currency: Currency{
				ID:       it.CurrencyID,
				Currency: it.Currency.String,
				Symbol:   it.Symbol.String,
			},
		}
	}
	return items
}
