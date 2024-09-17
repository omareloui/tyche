package models

import (
	"fmt"
	"time"

	"github.com/omareloui/financial-planner/internal/db"
)

type FrequencyBy uint8

const (
	Day FrequencyBy = iota
	Month
	Year
)

func (fb FrequencyBy) String() string {
	return [...]string{"Day", "Month", "Year"}[fb]
}

type Transaction struct {
	ID          string  `validate:"required"`
	Amount      float64 `validate:"required,min=1"`
	Currency    Currency
	Kind        TransactionKind
	Description string
	FulfilledOn time.Time `validate:"omitempty"`
	ScheduledTo time.Time `validate:"required"`
	CreatedAt   time.Time
}

type FrequentTransaction struct {
	ID                       string          `validate:"required"`
	FrequencyBy              FrequencyBy     `validate:"required"`
	Frequency                uint            `validate:"required,min=1"`
	Amount                   float64         `validate:"required,min=1"`
	Currency                 Currency        `validate:"required"`
	Kind                     TransactionKind `validate:"required"`
	Description              string
	CreatedAt                time.Time `validate:"required"`
	LastTransactionCreatedAt time.Time `validate:"required"`
}

func (t *Transaction) ViewAmount() string {
	return formatAmount(t.Amount, t.Currency.Enum)
}

func (ft *FrequentTransaction) ViewFrequency() string {
	return fmt.Sprintf("Each %d %s", ft.Frequency, ft.FrequencyBy)
}

func (ft *FrequentTransaction) ViewAmount() string {
	return formatAmount(ft.Amount, ft.Currency.Enum)
}

func DBTransactionRowToTransaction(dbTx *db.GetTransactionRow) *Transaction {
	return &Transaction{
		ID:          dbTx.ID,
		Amount:      dbTx.Amount,
		Description: dbTx.Description.String,
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
