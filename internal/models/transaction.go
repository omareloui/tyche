package models

import (
	"time"

	"github.com/omareloui/financial-planner/internal/db"
)

type Transaction struct {
	ID          string  `validate:"required"`
	Amount      float64 `validate:"required,min=1"`
	Currency    Currency
	Kind        TransactionKind
	Description string
	IsRecurrent bool
	FulfilledOn time.Time `validate:"omitempty"`
	ScheduledTo time.Time `validate:"required"`
	CreatedAt   time.Time
}

func (t *Transaction) ViewAmount() string {
	return formatAmount(t.Amount, t.Currency.Enum)
}

func DBTransactionRowToTransaction(dbTx *db.GetTransactionRow) *Transaction {
	return &Transaction{
		ID:          dbTx.ID,
		Amount:      dbTx.Amount,
		Description: dbTx.Description.String,
		IsRecurrent: dbTx.IsRecurrent,
		FulfilledOn: dbTx.FulfilledOn.Time,
		ScheduledTo: dbTx.ScheduledTo,
		CreatedAt:   dbTx.CreatedAt.(time.Time),
		Currency: Currency{
			ID:        dbTx.CurrencyID,
			Enum:      CurrencyEnum(dbTx.Enum.Int64),
			EgpFxRate: dbTx.EgpFxRate.Float64,
		},
		Kind: TransactionKind{
			ID:              dbTx.KindID,
			Kind:            dbTx.Kind.String,
			IncreaseBalance: dbTx.IncreaseBalance.Bool,
		},
	}
}

func DBTransactionsRowToTransaction(dbTx *db.GetTransactionsRow) *Transaction {
	return &Transaction{
		ID:          dbTx.ID,
		Amount:      dbTx.Amount,
		Description: dbTx.Description.String,
		IsRecurrent: dbTx.IsRecurrent,
		FulfilledOn: dbTx.FulfilledOn.Time,
		ScheduledTo: dbTx.ScheduledTo,
		CreatedAt:   dbTx.CreatedAt.(time.Time),
		Currency: Currency{
			ID:        dbTx.CurrencyID,
			Enum:      CurrencyEnum(dbTx.Enum.Int64),
			EgpFxRate: dbTx.EgpFxRate.Float64,
		},
		Kind: TransactionKind{
			ID:              dbTx.KindID,
			Kind:            dbTx.Kind.String,
			IncreaseBalance: dbTx.IncreaseBalance.Bool,
		},
	}
}
