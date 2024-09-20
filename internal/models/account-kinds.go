package models

type AccountKindEnum uint8

const (
	CreditCard AccountKindEnum = iota + 1
	BankAccount
	Debt
	Cash
)

func (a AccountKindEnum) String() string {
	return [...]string{
		"Credit Card", "Bank Account",
		"Debt", "Cash",
	}[a-1]
}

type AccountKind struct {
	ID   string          `validate:"required"`
	Enum AccountKindEnum `validate:"required"`
}
