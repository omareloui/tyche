package models

const (
	ExpenseTxKind  = "EXPENSE"
	IncomeTxKind   = "INCOME"
	TransferTxKind = "TRANSFER"
	DebtTxKind     = "DEBT"
)

type TransactionKind struct {
	ID              string `validate:"required"`
	Kind            string `conform:"trim,upper"`
	IncreaseBalance bool
}
