package models

import "errors"

// When the user buys something for someone else, the expense stays a plain
// one (its account, merchant, lines and receipt) flagged ForOthers, with who
// it was for (ForPerson, a non-merchant counterparty), and is left out of
// every analysis: a budget, a category's or a merchant's spending only count
// what the user bought for themselves. Gift or not: when they'll pay it back,
// an income reimbursing it is scheduled (like a split bill's payments), which
// counts 0 too.

var (
	ErrForOthersKind     = errors.New("only an expense of yours (not a goal's, a loan's payment, a transfer's leg nor a reimbursement) can be bought for someone")
	ErrForOthersBoughtBy = errors.New("something someone bought for you can't also be for someone else")
	ErrForOthersSplit    = errors.New("a split bill can't be bought for someone, its shares already are theirs")
	ErrForOthersPerson   = errors.New("pick who it's for")
)

// CanBeForOthers reports whether the expense can be marked bought for
// someone (or is already).
func (t *Transaction) CanBeForOthers() bool {
	return t.Kind == ExpenseTxKind && !t.IsLocked() && t.Goal == nil &&
		!t.IsTransferLeg() && !t.IsLoanPayment() && !t.IsReimbursement()
}

// ValidateForOthers checks the expense can be marked bought for someone.
func ValidateForOthers(t *Transaction) error {
	if !t.CanBeForOthers() {
		return ErrForOthersKind
	}
	if len(t.BoughtBy) > 0 {
		return ErrForOthersBoughtBy
	}
	if t.SplitPeople > 0 {
		return ErrForOthersSplit
	}
	return nil
}

// ForPersonName is who it was bought for, "" when unnamed.
func (t *Transaction) ForPersonName() string {
	if t.ForPerson == nil {
		return ""
	}
	return t.ForPerson.Name
}

// ForOthersLabel names who it was bought for, e.g. "For Sara".
func (t *Transaction) ForOthersLabel() string {
	if name := t.ForPersonName(); name != "" {
		return "For " + name
	}
	return "For someone"
}

// ForOthersOwed is what's left for them to pay back: the amount less the
// incomes reimbursing it (scheduled or not).
func (t *Transaction) ForOthersOwed() float64 {
	return t.Amount - t.Reimbursed
}

// ForOthersDescription names the income paying it back after the expense.
func ForOthersDescription(t *Transaction) string {
	switch {
	case t.Description != "":
		return "Paid back: " + t.Description
	case t.Counterparty != nil && t.Counterparty.Name != "":
		return "Paid back: " + t.Counterparty.Name
	default:
		return "Paid back"
	}
}
