package models

type Account struct {
	ID                 string `validate:"required"`
	Amount             float64
	Currency           Currency
	Kind               AccountKind
	IBAN               string `validate:"omitempty,iban"`
	CardNumber         string `validate:"omitempty,credit_card"`
	Note               string
	IsArchive          bool
	ExcludeFromBalance bool
}

func (a Account) MaskIBAN() string {
	rs := []rune(a.IBAN)
	for i := 0; i < len(rs)-4; i++ {
		rs[i] = '*'
	}
	return string(rs)
}
