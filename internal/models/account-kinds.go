package models

type AccKind uint8

const (
	AccKindNil AccKind = iota
	AccKindCreditCard
	AccKindBankAcc
	AccKindCash
	AccKindDebt
	// AccKindGamyaa is a rotating savings group (gam'yaa), see Gamyaa.
	AccKindGamyaa
)

const DfltAccKind = AccKindBankAcc

func (a AccKind) String() string {
	return [...]string{
		"???", "Credit Card",
		"Bank Account",
		"Cash", "Debt", "Gam'yaa",
	}[a]
}

type AccountKind struct {
	ID   string `validate:"required"`
	Enum AccKind
}
