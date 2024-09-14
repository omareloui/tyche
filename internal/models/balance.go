package models

import (
	"github.com/omareloui/financial-planner/internal/db"
)

type Balance struct {
	ID       string  `validate:"required"`
	Amount   float64 `validate:"require"`
	Currency Currency
}

func (b *Balance) ViewAmount() string {
	return formatAmount(b.Amount, b.Currency)
}

func DBBalanceToBalance(dbBalance *db.GetBalancesRow) *Balance {
	return &Balance{
		ID:     dbBalance.ID,
		Amount: dbBalance.Amount,
		Currency: Currency{
			ID:       dbBalance.CurrencyID,
			Currency: dbBalance.Currency.String,
			Symbol:   dbBalance.Symbol.String,
		},
	}
}

func DBBalancesToBalances(dbBalance []db.GetBalancesRow) []Balance {
	balances := make([]Balance, len(dbBalance))
	for i, cur := range dbBalance {
		balances[i] = *DBBalanceToBalance(&cur)
	}
	return balances
}
