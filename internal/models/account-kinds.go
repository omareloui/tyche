package models

type AccountKindEnum uint8

const (
	AccountKindNil AccountKindEnum = iota
	AccountKindCreditCard
	AccountKindBankAccount
	AccountKindDebtCard
	AccountKindCash
	AccountKindDebt
)

const DfltAccountKind = AccountKindCreditCard

func (a AccountKindEnum) String() string {
	return [...]string{
		"???", "Credit Card", "Bank Account",
		"Debt Card", "Cash", "Debt",
	}[a]
}

type AccountKind struct {
	ID   string `validate:"required"`
	Enum AccountKindEnum
}
