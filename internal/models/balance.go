package models

type Balance struct {
	ID       string  `validate:"required"`
	Amount   float64 `validate:"require"`
	Currency Currency
}

func (b *Balance) ViewAmount() string {
	return formatAmount(b.Amount, b.Currency.Enum)
}

func (b *Balance) ViewEgpAmount() string {
	return formatAmount(b.Currency.EgpFxRate*b.Amount, CurrencyEgp)
}

func (b *Balance) EgpAmount() float64 {
	return b.Currency.EgpFxRate * b.Amount
}
