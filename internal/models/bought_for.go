package models

import (
	"errors"
	"strconv"
	"time"
)

// When someone buys the user something, the user owes them its price: a debt
// borrowed from them (out of their Debt account into one of the user's) and
// the expense of what they bought (out of that account). The debt points at
// the expense (Transaction.BoughtForID) and is listed under it. Several
// people can chip in, what their debts brought in being at most the expense.

// BoughtForWindow is how far apart, either way, a debt and the expense it
// bought are offered to be linked.
const BoughtForWindow = 14 * 24 * time.Hour

var (
	ErrBoughtForKinds    = errors.New("only a debt borrowed from someone can pay for an expense")
	ErrBoughtForExpense  = errors.New("goal expenses, loan payments and transfers between your accounts can't be bought by someone")
	ErrBoughtForCurrency = errors.New("the debt must bring money into the expense's currency")
	ErrBoughtForExceeds  = errors.New("the debts would be more than the expense")
	ErrBoughtForLinked   = errors.New("the debt already paid for another expense")
)

// IsBorrow reports whether it's a debt borrowed from someone: out of their
// Debt account into one of the user's.
func (t *Transaction) IsBorrow() bool {
	return t.Kind == DebtTxKind && t.Account.Kind.Enum == AccKindDebt &&
		t.ToAccount != nil && t.ToAccount.Kind.Enum != AccKindDebt
}

// IsBoughtFor reports whether it's the debt of something bought for the
// user.
func (t *Transaction) IsBoughtFor() bool {
	return t.BoughtForID != ""
}

// CanBeBoughtBy reports whether the expense can be linked to the debts of the
// people who bought it: an expense of the user's, not a goal's, a loan's
// payment, nor a transfer's leg.
func (t *Transaction) CanBeBoughtBy() bool {
	return t.Kind == ExpenseTxKind && !t.IsLocked() && t.Goal == nil &&
		!t.IsTransferLeg() && !t.IsLoanPayment()
}

// CanPayFor reports whether the debt can be linked to an expense it paid for.
func (t *Transaction) CanPayFor() bool {
	return t.IsBorrow() && !t.IsLocked() && !t.IsBoughtFor()
}

// BoughtByTotal is what its debts brought in, in its account's currency.
func (t *Transaction) BoughtByTotal() float64 {
	var total float64
	for _, d := range t.BoughtBy {
		total += d.ToAmount
	}
	return total
}

// DebtPartyName is who a debt is with: its Debt account's counterparty, else
// the account's name.
func DebtPartyName(acc *Account) string {
	if acc.Counterparty != nil && acc.Counterparty.Name != "" {
		return acc.Counterparty.Name
	}
	return acc.Name
}

// BoughtByLabel names who bought an expense, e.g. "Bought by Sara" or
// "Bought by Sara +2".
func BoughtByLabel(debts []Transaction) string {
	if len(debts) == 0 {
		return ""
	}
	label := "Bought by " + DebtPartyName(&debts[0].Account)
	if n := len(debts) - 1; n > 0 {
		label += " +" + strconv.Itoa(n)
	}
	return label
}

// ValidateBoughtFor checks the debt can be linked to the expense, the other
// debts of the expense bringing in others already.
func ValidateBoughtFor(debt, expense *Transaction, others float64) error {
	if !debt.IsBorrow() || debt.IsLocked() || expense.Kind != ExpenseTxKind {
		return ErrBoughtForKinds
	}
	if !expense.CanBeBoughtBy() {
		return ErrBoughtForExpense
	}
	if debt.IsBoughtFor() && debt.BoughtForID != expense.ID {
		return ErrBoughtForLinked
	}
	if debt.ToAccount.Currency.ID != expense.Account.Currency.ID {
		return ErrBoughtForCurrency
	}
	if others+debt.ToAmount > expense.Amount+boughtForEpsilon {
		return ErrBoughtForExceeds
	}
	return nil
}

// boughtForEpsilon absorbs float rounding of amounts rounded to cents.
const boughtForEpsilon = 0.005

// BoughtForCandidate is an expense a debt can be linked to.
type BoughtForCandidate struct {
	ID            string
	Title         string
	Subtitle      string
	Category      string
	CategoryColor string
	Account       string
	Amount        float64
	BoughtBy      float64
	Currency      Currency
	ScheduledTo   time.Time
}

func (e *BoughtForCandidate) ViewAmount() string {
	return e.Currency.Format(e.Amount)
}

// ViewLeft is what's left of it not bought by someone yet.
func (e *BoughtForCandidate) ViewLeft() string {
	return e.Currency.Format(e.Amount - e.BoughtBy)
}
