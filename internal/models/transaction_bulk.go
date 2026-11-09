package models

import (
	"fmt"
	"slices"
	"strings"
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
	// Review marks them reviewed or sends them back to review.
	Review TxBulkReview
}

// TxBulkReview is what a bulk edit does to the transactions' review.
type TxBulkReview string

const (
	TxBulkReviewKeep  TxBulkReview = ""
	TxBulkReviewDone  TxBulkReview = "reviewed"
	TxBulkReviewNeeds TxBulkReview = "needs"
)

// ParseTxBulkReview is the review of a bulk edit form's value, false when
// it's none of them.
func ParseTxBulkReview(s string) (TxBulkReview, bool) {
	switch r := TxBulkReview(s); r {
	case TxBulkReviewKeep, TxBulkReviewDone, TxBulkReviewNeeds:
		return r, true
	}
	return TxBulkReviewKeep, false
}

// TxBulkSkip is why (part of) a bulk edit was left out of a transaction.
type TxBulkSkip string

const (
	TxBulkSkipLocked       TxBulkSkip = "locked"
	TxBulkSkipCategory     TxBulkSkip = "category doesn't fit"
	TxBulkSkipCounterparty TxBulkSkip = "takes no counterparty"
	TxBulkSkipAhead        TxBulkSkip = "fulfilled can't be ahead"
)

// TxBulkLock is why a transaction can't be bulk edited at all, empty when
// it can.
type TxBulkLock string

const (
	TxBulkLockNone       TxBulkLock = ""
	TxBulkLockAdjustment TxBulkLock = "adjustment"
	TxBulkLockGoal       TxBulkLock = "goal"
	TxBulkLockDeleted    TxBulkLock = "deleted"
)

// BulkLock is why tx can't be bulk edited: a balance correction (signed,
// made by fixing balances), a goal's (made by its actions) or a deleted
// account's.
func (t *Transaction) BulkLock() TxBulkLock {
	switch {
	case t.Kind == AdjustmentTxKind:
		return TxBulkLockAdjustment
	case t.IsGoalTx():
		return TxBulkLockGoal
	case t.IsLocked():
		return TxBulkLockDeleted
	}
	return TxBulkLockNone
}

// DescribeBulkLocks tells how many of each lock there are, e.g. "2 balance
// corrections, 1 goal transaction".
func DescribeBulkLocks(counts map[TxBulkLock]int) string {
	names := []struct {
		lock         TxBulkLock
		one, several string
	}{
		{TxBulkLockAdjustment, "balance correction", "balance corrections"},
		{TxBulkLockGoal, "goal transaction", "goal transactions"},
		{TxBulkLockDeleted, "transaction of a deleted account", "transactions of deleted accounts"},
	}
	var parts []string
	for _, n := range names {
		switch c := counts[n.lock]; {
		case c == 1:
			parts = append(parts, "1 "+n.one)
		case c > 1:
			parts = append(parts, fmt.Sprintf("%d %s", c, n.several))
		}
	}
	return strings.Join(parts, ", ")
}

func (e *TxBulkEdit) IsZero() bool {
	return e.Category == nil && !e.ClearCategory && e.Counterparty == nil &&
		!e.ClearCounterparty && e.Date == nil && e.Review == TxBulkReviewKeep
}

// Apply is tx with the edit, and why parts of it were skipped (the whole of
// it for locked ones). today is the user's, as Handler.userToday: a fulfilled
// transaction can't move ahead of it.
func (e *TxBulkEdit) Apply(tx *Transaction, today time.Time) (*Transaction, []TxBulkSkip) {
	if tx.BulkLock() != TxBulkLockNone {
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
