package models

import (
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/formatters"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

type Transaction struct {
	ID             string
	Account        Account
	Amount         float64
	Kind           TxKind
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
	NeedsReview    bool
	FromSMS        bool
	SMSBody        string
	CreatedAt      time.Time
	// ReimbursesID is the expense this income pays back (part of).
	ReimbursesID string
	// Reimbursed is the total of the incomes reimbursing this expense.
	Reimbursed     float64
	Reimbursements []Transaction
}

type commonFieldsForTxAndRtx struct {
	ID             string  `validate:"required,ulid"`
	AccountID      string  `validate:"required,ulid"`
	Amount         float64 `validate:"required"`
	Kind           uint8   `validate:"required"`
	CategoryID     string  `validate:"excluded_if=Kind 3,excluded_if=Kind 4,omitempty,ulid"`
	CounterpartyID string  `validate:"excluded_if=Kind 3,excluded_if=Kind 4,omitempty,ulid"`
	ToAccountID    string  `validate:"required_if=Kind 3,required_if=Kind 4,omitempty,ulid"`
	ToAmount       float64 `validate:"required_if=Kind 3,required_if=Kind 4"`
	Description    string  `conform:"trim"`
	GoalID         string  `validate:"excluded_if=Kind 2,excluded_if=Kind 3,excluded_if=Kind 4,excluded_if=Kind 5,omitempty,ulid"`
}

type createTransaction struct {
	commonFieldsForTxAndRtx
	ScheduledTo time.Time `validate:"required"`
}

func (t *Transaction) ViewAmount() string {
	return formatters.FormatAmount(t.Amount, t.Account.Currency.Symbol)
}

func (t *Transaction) ViewNetAmount() string {
	return formatters.FormatAmount(t.NetAmount(), t.Account.Currency.Symbol)
}

// IsReimbursement reports whether the transaction pays back (part of) an
// expense.
func (t *Transaction) IsReimbursement() bool {
	return t.ReimbursesID != ""
}

// NetAmount is the amount minus what got reimbursed of it.
func (t *Transaction) NetAmount() float64 {
	return t.Amount - t.Reimbursed
}

// AnalysisAmount is the amount every analysis (spending, income, charts) must
// use: expenses count net of their reimbursements and the reimbursements
// themselves count as 0, as they're already netted from their expense.
func (t *Transaction) AnalysisAmount() float64 {
	if t.IsReimbursement() {
		return 0
	}
	return t.NetAmount()
}

func (t *Transaction) ViewToAmount() string {
	if t.ToAccount == nil {
		return ""
	}
	return formatters.FormatAmount(t.ToAmount, t.ToAccount.Currency.Symbol)
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
			Kind:           uint8(t.Kind),
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
