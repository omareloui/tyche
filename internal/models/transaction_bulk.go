package models

import (
	"slices"
	"time"
)

// TxBulkEdit is what a bulk edit of the transactions list changes; nil
// fields are kept as they are.
type TxBulkEdit struct {
	// Category is set on the transactions whose kind it allows, ClearCategory
	// removes it instead.
	Category      *Category
	ClearCategory bool
	// Counterparty is set, ClearCounterparty removes it instead.
	Counterparty      *Counterparty
	ClearCounterparty bool
	// Date is the day to move them to, their time of day kept.
	Date *time.Time
	// Reviewed marks the ones needing review as reviewed.
	Reviewed bool
}

// TxBulkSkip is why (part of) a bulk edit was left out of a transaction.
type TxBulkSkip string

const (
	TxBulkSkipLocked       TxBulkSkip = "locked"
	TxBulkSkipCategory     TxBulkSkip = "category doesn't fit"
	TxBulkSkipCounterparty TxBulkSkip = "takes no counterparty"
	TxBulkSkipAhead        TxBulkSkip = "fulfilled can't be ahead"
)

func (e *TxBulkEdit) IsZero() bool {
	return e.Category == nil && !e.ClearCategory && e.Counterparty == nil &&
		!e.ClearCounterparty && e.Date == nil && !e.Reviewed
}

// Apply is tx with the edit, and why parts of it were skipped (the whole of
// it for locked ones). today is the user's, as Handler.userToday: a fulfilled
// transaction can't move ahead of it.
func (e *TxBulkEdit) Apply(tx *Transaction, today time.Time) (*Transaction, []TxBulkSkip) {
	if tx.IsLocked() || tx.Kind == AdjustmentTxKind {
		return nil, []TxBulkSkip{TxBulkSkipLocked}
	}
	out := *tx
	var skips []TxBulkSkip
	// Transfers and debts take no category nor counterparty (Transaction's
	// validation).
	noRefs := tx.Kind == TransferTxKind || tx.Kind == DebtTxKind

	switch {
	case e.ClearCategory:
		out.Category = nil
	case e.Category != nil:
		if noRefs || !slices.Contains(e.Category.Kinds, tx.Kind.CategoryKind()) {
			skips = append(skips, TxBulkSkipCategory)
		} else {
			out.Category = e.Category
		}
	}

	switch {
	case e.ClearCounterparty:
		out.Counterparty = nil
	case e.Counterparty != nil:
		if noRefs {
			skips = append(skips, TxBulkSkipCounterparty)
		} else {
			out.Counterparty = e.Counterparty
		}
	}

	if e.Date != nil {
		if !tx.FulfilledAt.IsZero() && e.Date.After(today) {
			skips = append(skips, TxBulkSkipAhead)
		} else {
			out.ScheduledTo = KeepTimeOfDay(*e.Date, tx.ScheduledTo)
		}
	}
	return &out, skips
}

// KeepTimeOfDay is date at the time of day of timeOf, e.g. to keep a
// transaction's order within its day when only its day is picked.
func KeepTimeOfDay(date, timeOf time.Time) time.Time {
	if date.IsZero() || timeOf.IsZero() {
		return date
	}
	y, m, d := date.Date()
	return time.Date(y, m, d, timeOf.Hour(), timeOf.Minute(), timeOf.Second(), timeOf.Nanosecond(), timeOf.Location())
}
