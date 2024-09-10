package models

import (
	"time"

	"github.com/omareloui/financial-plans/internal/db"
)

type Transaction struct {
	ID                string
	Amount            int64
	CurrencyID        string
	TransactionKindID string
	CanRepeat         bool
	FulfilledOn       time.Time
	ScheduledTo       time.Time
	CreatedAt         time.Time

	Kind     string
	Currency string
}

func DBTransactionToTransaction(dbTransaction db.Transaction) *Transaction {
	return &Transaction{
		ID:                dbTransaction.ID,
		Amount:            dbTransaction.Amount,
		CurrencyID:        dbTransaction.CurrencyID,
		TransactionKindID: dbTransaction.TransactionKindID,
		CanRepeat:         dbTransaction.CanRepeat,
		FulfilledOn:       dbTransaction.FulfilledOn.Time,
		ScheduledTo:       dbTransaction.ScheduledTo.Time,
		// TODO: test
		CreatedAt: dbTransaction.CreatedAt.(time.Time),
	}
}
