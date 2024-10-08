package models

type TxKind uint8

const (
	NilTxKind TxKind = iota
	ExpenseTxKind
	IncomeTxKind
	TransferTxKind
	DebtTxKind
)

func (tk TxKind) String() string {
	return [...]string{
		"???", "Expense", "Income",
		"Transfer", "Debt",
	}[tk]
}

type TransactionKind struct {
	ID              string `validate:"required"`
	Enum            TxKind `conform:"trim,upper"`
	IncreaseBalance bool
}
