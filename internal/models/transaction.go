package models

import (
	"strings"
	"time"

	"github.com/omareloui/financial-plans/internal/db"
)

type Transaction struct {
	ID          string `validate:"required"`
	Amount      int64  `validate:"required,min=1"`
	IsRecurrent bool
	FulfilledOn time.Time `validate:"omitempty"`
	ScheduledTo time.Time `validate:"required"`
	CreatedAt   time.Time
	Kind        TransactionKind
	Currency    Currency
}

func (t *Transaction) ViewAmount() string {
	strBuilder := new(strings.Builder)
	if t.Currency.Currency != "" {
		strBuilder.WriteString(t.Currency.Symbol)
		strBuilder.WriteString(" ")
	}
	strBuilder.WriteString(formatNum(t.Amount))
	return strBuilder.String()
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

func DBTransactionRawToTransaction(dbTransaction *db.GetTransactionRow) *Transaction {
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
			ID:   dbTransaction.KindID,
			Kind: dbTransaction.Kind.String,
		},
	}
}

func DBTransactionsRawToTransaction(dbTransaction *db.GetTransactionsRow) *Transaction {
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
			ID:   dbTransaction.KindID,
			Kind: dbTransaction.Kind.String,
		},
	}
}
