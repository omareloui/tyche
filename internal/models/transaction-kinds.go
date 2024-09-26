package models

const ExpensesTxKind = "EXPENSES"

type TransactionKind struct {
	ID              string `validate:"required"`
	Kind            string `conform:"trim,upper"`
	IncreaseBalance bool
}
