package models

type AccKind uint8

const (
	AccKindNil AccKind = iota
	AccKindCreditCard
	AccKindBankAcc
	AccKindCash
	AccKindDebt
)

const DfltAccKind = AccKindBankAcc

func (a AccKind) String() string {
	return [...]string{
		"???", "Credit Card",
		"Bank Account",
		"Cash", "Debt",
	}[a]
}

type AccountKind struct {
	ID   string `validate:"required"`
	Enum AccKind
}
