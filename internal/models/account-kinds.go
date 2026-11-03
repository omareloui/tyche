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
	// AccKindGoal holds the money put towards a goal, see Goal. It's hidden:
	// only the goal's actions touch it.
	AccKindGoal
)

const DfltAccKind = AccKindBankAcc

func (a AccKind) String() string {
	return [...]string{
		"???", "Credit Card",
		"Bank Account",
		"Cash", "Debt", "Gam'yaa",
		"Goal",
	}[a]
}

type AccountKind struct {
	ID   string `validate:"required"`
	Enum AccKind
}
