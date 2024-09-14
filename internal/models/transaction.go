package models

import (
	"time"

	"github.com/omareloui/financial-planner/internal/db"
)

type Transaction struct {
	ID          string  `validate:"required"`
	Amount      float64 `validate:"required,min=1"`
	IsRecurrent bool
	FulfilledOn time.Time `validate:"omitempty"`
	ScheduledTo time.Time `validate:"required"`
	CreatedAt   time.Time
	Kind        TransactionKind
	Currency    Currency
}

func (t *Transaction) ViewAmount() string {
	return formatAmount(t.Amount, t.Currency)
}

func DBTransactionToTransaction(dbTransaction *db.Transaction) *Transaction {
	return &Transaction{
		ID:          dbTransaction.ID,
		Amount:      dbTransaction.Amount,
		IsRecurrent: dbTransaction.IsRecurrent,
		FulfilledOn: dbTransaction.FulfilledOn.Time,
		ScheduledTo: dbTransaction.ScheduledTo,
		CreatedAt:   dbTransaction.CreatedAt.(time.Time),
		Kind: TransactionKind{
			ID: dbTransaction.KindID,
		},
		Currency: Currency{
			ID: dbTransaction.CurrencyID,
		},
	}
}

func DBTransactionRowToTransaction(dbTransaction *db.GetTransactionRow) *Transaction {
	return &Transaction{
		ID:          dbTransaction.ID,
		Amount:      dbTransaction.Amount,
		IsRecurrent: dbTransaction.IsRecurrent,
		FulfilledOn: dbTransaction.FulfilledOn.Time,
		ScheduledTo: dbTransaction.ScheduledTo,
		CreatedAt:   dbTransaction.CreatedAt.(time.Time),
		Currency: Currency{
			ID:       dbTransaction.CurrencyID,
			Currency: dbTransaction.Currency.String,
			Symbol:   dbTransaction.Symbol.String,
		},
		Kind: TransactionKind{
			ID:              dbTransaction.KindID,
			Kind:            dbTransaction.Kind.String,
			IncreaseBalance: dbTransaction.IncreaseBalance.Bool,
		},
	}
}

func DBTransactionsRowToTransaction(dbTransaction *db.GetTransactionsRow) *Transaction {
	return &Transaction{
		ID:          dbTransaction.ID,
		Amount:      dbTransaction.Amount,
		IsRecurrent: dbTransaction.IsRecurrent,
		FulfilledOn: dbTransaction.FulfilledOn.Time,
		ScheduledTo: dbTransaction.ScheduledTo,
		CreatedAt:   dbTransaction.CreatedAt.(time.Time),
		Currency: Currency{
			ID:       dbTransaction.CurrencyID,
			Currency: dbTransaction.Currency.String,
			Symbol:   dbTransaction.Symbol.String,
		},
		Kind: TransactionKind{
			ID:              dbTransaction.KindID,
			Kind:            dbTransaction.Kind.String,
			IncreaseBalance: dbTransaction.IncreaseBalance.Bool,
		},
	}
}
