package models

import (
	"time"
)

type Transaction struct {
	ID             string `validate:"required"`
	Account        Account
	Amount         float64 `validate:"required,min=1"`
	Kind           TransactionKind
	Description    string
	GoalTitle      string
	GoalPercentage float64
	FulfilledOn    time.Time
	ScheduledTo    time.Time `validate:"required"`
	CreatedAt      time.Time
}

func (t *Transaction) ViewAmount() string {
	return formatAmount(t.Amount, t.Account.Currency.Enum)
}
