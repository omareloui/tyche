package models

type AccountKindEnum uint8

const (
	AccountKindCreditCard AccountKindEnum = iota + 1
	AccountKindBankAccount
	AccountKindDebtCard
	AccountKindCash
)

const DfltAccountKind = AccountKindCreditCard

func (a AccountKindEnum) String() string {
	return [...]string{
		"Credit Card", "Bank Account",
		"Debt Card", "Cash",
	}[a-1]
}

type AccountKind struct {
	ID   string `validate:"required"`
	Enum AccountKindEnum
}
