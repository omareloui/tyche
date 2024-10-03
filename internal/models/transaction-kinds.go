package models

const (
	ExpenseTxKind  = "EXPENSE"
	IncomeTxKind   = "INCOME"
	TransferTxKind = "TRANSFER"
)

type TransactionKind struct {
	ID              string `validate:"required"`
	Kind            string `conform:"trim,upper"`
	IncreaseBalance bool
}
