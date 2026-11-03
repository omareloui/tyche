package models

import (
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/numsutils"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

// Goal is something to save for. Its money sits in its own hidden account
// (AccKindGoal): putting money towards it is a transfer to the account,
// taking some back one from it, and buying it an expense out of it.
type Goal struct {
	ID       string  `validate:"required"`
	Name     string  `validate:"required" conform:"trim,titlecase"`
	Price    float64 `validate:"required,gt=0"`
	Currency Currency
	// Account holds the money saved, in the goal's currency.
	Account *Account
	// Purchased is what buying it took out of its account.
	Purchased    float64
	FulfilledAt  time.Time
	CreatedAt    time.Time
	Transactions []Transaction
	// Plan is how it's saved for, nil without one.
	Plan *GoalPlan
	// Loan is the payments of the loan it was bought with, in its currency.
	Loan GoalLoan
}

// GoalLoan is the payments of the loan a goal was bought with.
type GoalLoan struct {
	Payments, PaidPayments int
	Total, Paid            float64
}

// HasLoan tells if it was bought with a loan, its payments scheduled.
func (gl *Goal) HasLoan() bool {
	return gl.Loan.Payments > 0
}

// PayingOff tells if it's bought with a loan not paid off yet.
func (gl *Goal) PayingOff() bool {
	return !gl.FulfilledAt.IsZero() && gl.HasLoan() && gl.Loan.PaidPayments < gl.Loan.Payments
}

// PayoffPercentage is how much of what buying it costs is paid: what it
// took on the day and the loan's payments made, of those and the rest.
func (gl *Goal) PayoffPercentage() float64 {
	total := gl.Purchased + gl.Loan.Total
	if total <= 0 {
		return 0
	}
	return (gl.Purchased + gl.Loan.Paid) / total * 100
}

// ShownPercentage is the saving progress, or the paying off once bought
// with a loan.
func (gl *Goal) ShownPercentage() float64 {
	if !gl.FulfilledAt.IsZero() && gl.HasLoan() {
		return gl.PayoffPercentage()
	}
	return gl.ProgressPercentage()
}

// GoalAccountPrefix starts the name of the goals' accounts.
const GoalAccountPrefix = "Goal: "

// GoalAccountName is the name of the account of the goal named name.
func GoalAccountName(name string) string {
	return GoalAccountPrefix + name
}

type createGoal struct {
	ID         string  `validate:"required"`
	Name       string  `validate:"required" conform:"trim,titlecase"`
	Price      float64 `validate:"required,gt=0"`
	CurrencyID string  `validate:"required"`
}

func (gl *Goal) ViewAmount() string {
	return gl.Currency.Format(gl.Price)
}

// Progress is the money saved: its account's balance plus what buying it
// took out of it.
func (gl *Goal) Progress() float64 {
	if gl.Account == nil {
		return gl.Purchased
	}
	return gl.Account.Amount + gl.Purchased
}

// Balance is the money still in its account (left after buying it).
func (gl *Goal) Balance() float64 {
	if gl.Account == nil {
		return 0
	}
	return gl.Account.Amount
}

func (gl *Goal) ViewBalance() string {
	return gl.Currency.Format(gl.Balance())
}

func (gl *Goal) ProgressPercentage() float64 {
	return gl.Progress() / gl.Price * 100
}

func (gl *Goal) Remaining() float64 {
	return gl.Price - gl.Progress()
}

func (gl *Goal) ViewRemaining() string {
	return gl.Currency.Format(gl.Remaining())
}

func (gl *Goal) ViewProgress() string {
	return gl.Currency.Format(gl.Progress())
}

// MonthlyFor is what to put aside every month to get there in
// numberOfMonths.
func (gl *Goal) MonthlyFor(numberOfMonths int) float64 {
	return numsutils.Round(gl.Remaining() / float64(numberOfMonths))
}

func (gl *Goal) toValidate() *createGoal {
	return &createGoal{
		ID:         gl.ID,
		Name:       gl.Name,
		Price:      gl.Price,
		CurrencyID: gl.Currency.ID,
	}
}

func (gl *Goal) SanitizeAndValidate(sani *sanitizer.Sanitizer, valid *valid.Validator) error {
	glToValidate := gl.toValidate()
	if err := sani.SanitizeStruct(glToValidate); err != nil {
		return errs.ErrSanitizer
	}
	if valerr := valid.Validate(glToValidate); valerr != nil {
		return *valerr
	}
	return nil
}
