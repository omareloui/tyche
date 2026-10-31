package models

import "encoding/json"

type TxKind uint8

const (
	NilTxKind TxKind = iota
	ExpenseTxKind
	IncomeTxKind
	TransferTxKind
	DebtTxKind
	// RefundTxKind is money returned for an earlier expense. It adds to the
	// account like an income but uses expense categories, and should reduce
	// the category's spending rather than count as income.
	RefundTxKind
	// AdjustmentTxKind corrects an account's balance to what it really is. Its
	// amount is signed (added to the account) and it's left out of every
	// analysis, as it's neither earned nor spent.
	AdjustmentTxKind
)

func (tk TxKind) String() string {
	return [...]string{
		"???", "Expense", "Income",
		"Transfer", "Debt", "Refund",
		"Adjustment",
	}[tk]
}

// IsIncoming reports whether the transaction adds to its account without
// coming from another account.
func (tk TxKind) IsIncoming() bool {
	return tk == IncomeTxKind || tk == RefundTxKind
}

// CategoryKind is the kind the transaction's category must allow.
func (tk TxKind) CategoryKind() TxKind {
	if tk == RefundTxKind {
		return ExpenseTxKind
	}
	return tk
}

func (tk TxKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(uint8(tk))
}

func (tk *TxKind) UnmarshalJSON(data []byte) error {
	var val uint8
	if err := json.Unmarshal(data, &val); err != nil {
		return err
	}
	*tk = TxKind(val)
	return nil
}

func GetTxKinds() []TxKind {
	return []TxKind{
		NilTxKind,
		ExpenseTxKind,
		IncomeTxKind,
		TransferTxKind,
		DebtTxKind,
		RefundTxKind,
		AdjustmentTxKind,
	}
}

func GetTxKindFromString(val string) TxKind {
	for _, kind := range GetTxKinds() {
		if kind.String() == val {
			return kind
		}
	}
	return NilTxKind
}
