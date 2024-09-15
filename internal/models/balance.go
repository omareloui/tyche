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
	return formatAmount(b.Amount, b.Currency.Enum)
}

func (b *Balance) ViewEgpAmount() string {
	return formatAmount(b.Currency.EgpFxRate*b.Amount, CurrencyEgp)
}

func (b *Balance) EgpAmount() float64 {
	return b.Currency.EgpFxRate * b.Amount
}

func DBBalanceToBalance(dbBalance *db.GetBalancesRow) *Balance {
	return &Balance{
		ID:     dbBalance.ID,
		Amount: dbBalance.Amount,
		Currency: Currency{
			ID:        dbBalance.CurrencyID,
			Enum:      CurrencyEnum(dbBalance.Enum.Int64),
			EgpFxRate: dbBalance.EgpFxRate.Float64,
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
