package models

import (
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

type Transaction struct {
	ID             string
	Account        Account
	Amount         float64
	Kind           TransactionKind
	Category       *Category
	ToAccount      *Account
	Counterparty   *Counterparty
	ToAmount       float64
	Description    string
	GoalName       string
	GoalPercentage float64
	ScheduledTo    time.Time
	FulfilledAt    time.Time
	CreatedAt      time.Time
}

type createTransaction struct {
	ID             string    `validate:"required,ulid"`
	AccountID      string    `validate:"required,ulid"`
	Amount         float64   `validate:"required"`
	KindID         string    `validate:"required,ulid"`
	Kind           TxKind    `validate:"required"`
	CategoryID     string    `validate:"excluded_if=Kind 3,excluded_if=Kind 4,omitempty,ulid"`
	CounterpartyID string    `validate:"excluded_if=Kind 3,excluded_if=Kind 4,omitempty,ulid"`
	ToAccountID    string    `validate:"required_if=Kind 3,required_if=Kind 4,omitempty,ulid"`
	ToAmount       float64   `validate:"required_if=Kind 3,required_if=Kind 4"`
	Description    string    `conform:"trim"`
	ScheduledTo    time.Time `validate:"required"`
}

func (t *Transaction) ViewAmount() string {
	return formatAmount(t.Amount, t.Account.Currency.Enum)
}

func (t *Transaction) ViewToAmount() string {
	if t.ToAccount == nil {
		return ""
	}
	return formatAmount(t.ToAmount, t.ToAccount.Currency.Enum)
}

func (t *Transaction) toValidate() *createTransaction {
	var catId, toAccId, cpId string

	if cat := t.Category; cat != nil {
		catId = cat.ID
	}
	if cp := t.Counterparty; cp != nil {
		cpId = cp.ID
	}
	if toAcc := t.ToAccount; toAcc != nil {
		toAccId = toAcc.ID
	}

	return &createTransaction{
		ID:             t.ID,
		AccountID:      t.Account.ID,
		Amount:         t.Amount,
		KindID:         t.Kind.ID,
		Kind:           t.Kind.Enum,
		CategoryID:     catId,
		CounterpartyID: cpId,
		ToAccountID:    toAccId,
		ToAmount:       t.ToAmount,
		Description:    t.Description,
		ScheduledTo:    t.ScheduledTo,
	}
}

func (t *Transaction) SanitizeAndValidate(sani *sanitizer.Sanitizer, valid *valid.Validator) error {
	tc := t.toValidate()
	if err := sani.SanitizeStruct(tc); err != nil {
		return errs.ErrSanitizer
	}
	if valerr := valid.Validate(tc); valerr != nil {
		return *valerr
	}
	return nil
}

func (t *Transaction) Percentage(from float64) float64 {
	return t.Amount * t.Account.Currency.FxRate(CurrencyEgp) / from
}
