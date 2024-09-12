package models

import (
	"time"

	"github.com/omareloui/financial-plans/internal/db"
)

type Transaction struct {
	ID          string `validate:"required"`
	Amount      int64  `validate:"required,min=1"`
	CurrencyID  string `validate:"required"`
	KindID      string `validate:"required"`
	IsRecurrent bool
	FulfilledOn time.Time `validate:"omitempty"`
	ScheduledTo time.Time `validate:"omitempty"`
	CreatedAt   time.Time `validate:"required"`

	Kind     string
	Currency string
}

func DBTransactionToTransaction(dbTransaction *db.Transaction) *Transaction {
	return &Transaction{
		ID:          dbTransaction.ID,
		Amount:      dbTransaction.Amount,
		CurrencyID:  dbTransaction.CurrencyID,
		KindID:      dbTransaction.KindID,
		IsRecurrent: dbTransaction.IsRecurrent,
		FulfilledOn: dbTransaction.FulfilledOn.Time,
		ScheduledTo: dbTransaction.ScheduledTo.Time,
		CreatedAt:   dbTransaction.CreatedAt.(time.Time),
	}
}

func DBTransactionRawToTransaction(dbTransaction *db.GetTransactionsRow) *Transaction {
	return &Transaction{
		ID:          dbTransaction.ID,
		Amount:      dbTransaction.Amount,
		CurrencyID:  dbTransaction.CurrencyID,
		KindID:      dbTransaction.KindID,
		IsRecurrent: dbTransaction.IsRecurrent,
		FulfilledOn: dbTransaction.FulfilledOn.Time,
		ScheduledTo: dbTransaction.ScheduledTo.Time,
		CreatedAt:   dbTransaction.CreatedAt.(time.Time),
	}
}
