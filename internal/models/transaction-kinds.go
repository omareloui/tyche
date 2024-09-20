package models

const DfltKind = "EXPENSES"

type TransactionKind struct {
	ID              string `validate:"required"`
	Kind            string `conform:"trim,upper"`
	IncreaseBalance bool
}
