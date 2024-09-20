package models

import (
	"fmt"
	"time"
)

type FrequencyBy uint8

const (
	Day FrequencyBy = iota + 1
	Month
	Year
)

func (fb FrequencyBy) String() string {
	return [...]string{"Day", "Month", "Year"}[fb-1]
}

func FrequencyByOptions() map[string]string {
	m := make(map[string]string, 3)
	m[fmt.Sprintf("%d", Day)] = Day.String()
	m[fmt.Sprintf("%d", Month)] = Month.String()
	m[fmt.Sprintf("%d", Year)] = Year.String()
	return m
}

type FrequentTx struct {
	ID                       string          `validate:"required"`
	FrequencyBy              FrequencyBy     `validate:"required"`
	Frequency                uint            `validate:"required,min=1"`
	Amount                   float64         `validate:"required,min=1"`
	Currency                 Currency        `validate:"required"`
	Kind                     TransactionKind `validate:"required"`
	Description              string
	StartsOn                 time.Time `validate:"required"`
	StopsOn                  time.Time `validate:"omitempty,gtfield=StartsOn"`
	CreatedAt                time.Time `validate:"required"`
	LastTransactionCreatedAt time.Time `validate:"required"`

	Transactions []Transaction
}

func (ft *FrequentTx) ViewFrequency() string {
	return fmt.Sprintf("Each %d %s", ft.Frequency, ft.FrequencyBy)
}

func (ft *FrequentTx) ViewAmount() string {
	return formatAmount(ft.Amount, ft.Currency.Enum)
}
