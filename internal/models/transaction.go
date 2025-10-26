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
	Goal           *Goal
	GoalPercentage float64
	RecurrentTx    *RecurrentTx
	ScheduledTo    time.Time
	FulfilledAt    time.Time
	CreatedAt      time.Time
}

type commonFieldsForTxAndRtx struct {
	ID             string  `validate:"required,ulid"`
	AccountID      string  `validate:"required,ulid"`
	Amount         float64 `validate:"required"`
	KindID         string  `validate:"required,ulid"`
	CategoryID     string  `validate:"excluded_if=Kind 3,excluded_if=Kind 4,omitempty,ulid"`
	CounterpartyID string  `validate:"excluded_if=Kind 3,excluded_if=Kind 4,omitempty,ulid"`
	ToAccountID    string  `validate:"required_if=Kind 3,required_if=Kind 4,omitempty,ulid"`
	ToAmount       float64 `validate:"required_if=Kind 3,required_if=Kind 4"`
	Description    string  `conform:"trim"`
	GoalID         string  `validate:"excluded_if=Kind 2,excluded_if=Kind 3,excluded_if=Kind 4,omitempty,ulid"`
}

type createTransaction struct {
	commonFieldsForTxAndRtx
	ScheduledTo time.Time `validate:"required"`
}

func (t *Transaction) ViewAmount() string {
	return formatAmount(t.Amount, t.Account.Currency.Symbol)
}

func (t *Transaction) ViewToAmount() string {
	if t.ToAccount == nil {
		return ""
	}
	return formatAmount(t.ToAmount, t.ToAccount.Currency.Symbol)
}

func (t *Transaction) toValidate() *createTransaction {
	var catID, toAccID, cpID, glID string

	if cat := t.Category; cat != nil {
		catID = cat.ID
	}
	if cp := t.Counterparty; cp != nil {
		cpID = cp.ID
	}
	if toAcc := t.ToAccount; toAcc != nil {
		toAccID = toAcc.ID
	}
	if gl := t.Goal; gl != nil {
		glID = gl.ID
	}

	return &createTransaction{
		commonFieldsForTxAndRtx: commonFieldsForTxAndRtx{
			ID:             t.ID,
			AccountID:      t.Account.ID,
			Amount:         t.Amount,
			KindID:         t.Kind.ID,
			CategoryID:     catID,
			CounterpartyID: cpID,
			ToAccountID:    toAccID,
			ToAmount:       t.ToAmount,
			Description:    t.Description,
			GoalID:         glID,
		},
		ScheduledTo: t.ScheduledTo,
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

func (t *Transaction) Percentage(of float64, ofCur string) float64 {
	return t.Amount * t.Account.Currency.FxRate(ofCur) / of
}
