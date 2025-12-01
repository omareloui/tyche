package models

import "encoding/json"

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
