package models

import (
	"errors"
	"time"

	"github.com/omareloui/tyche/internal/formatters"
)

var (
	ErrReimbursementKinds        = errors.New("only an income can reimburse an expense")
	ErrReimbursementGoal         = errors.New("goal expenses can't be reimbursed")
	ErrReimbursementCurrency     = errors.New("the income and the expense must be in the same currency")
	ErrReimbursementExceeds      = errors.New("the reimbursements would exceed the expense")
	ErrReimbursementTransferPair = errors.New("transfers between your accounts can't be reimbursed")
)

// ReimbursableExpense is an expense an income can be linked to as its
// reimbursement.
type ReimbursableExpense struct {
	ID             string
	Title          string
	Subtitle       string
	Account        string
	Amount         float64
	Reimbursed     float64
	CurrencySymbol string
	ScheduledTo    time.Time
}

func (e *ReimbursableExpense) ViewAmount() string {
	return formatters.FormatAmount(e.Amount, e.CurrencySymbol)
}

func (e *ReimbursableExpense) ViewNetAmount() string {
	return formatters.FormatAmount(e.Amount-e.Reimbursed, e.CurrencySymbol)
}
