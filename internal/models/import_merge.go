package models

import (
	"errors"
	"math"
	"slices"
	"strings"
	"time"
)

// ImportMergeWindow is how far apart (either way) an imported transaction
// and the one it's merged into can be.
const ImportMergeWindow = 7 * 24 * time.Hour

var (
	ErrImportMergeNotImported = errors.New("Only an imported transaction can be merged into another.")
	ErrImportMergeSame        = errors.New("Pick another transaction.")
	ErrImportMergeAccount     = errors.New("It must be of the same account.")
	ErrImportMergeKind        = errors.New("It must move the money the same way.")
	ErrImportMergeLocked      = errors.New("That transaction can't be changed.")
)

// mergeableKind is a kind an imported row can be: money out or in.
func mergeableKind(k TxKind) bool {
	return k == ExpenseTxKind || k == IncomeTxKind || k == RefundTxKind
}

// CanMergeImported tells whether imported (a row of a CSV or PDF import) can
// be merged into another transaction.
func (t *Transaction) CanMergeImported() bool {
	return t.FromImport && !t.FulfilledAt.IsZero() && mergeableKind(t.Kind) &&
		!t.IsLocked() && !t.IsTransferLeg() && !t.IsReimbursement()
}

// ValidateImportMerge tells whether the imported transaction can be merged
// into target: the same account, fulfilled, moving the money the same way,
// target not locked.
func ValidateImportMerge(imported, target *Transaction) error {
	switch {
	case !imported.CanMergeImported():
		return ErrImportMergeNotImported
	case imported.ID == target.ID, target.ImportID != "" && target.ImportID == imported.ImportID:
		// another row of the same file isn't what this one already is
		return ErrImportMergeSame
	case imported.Account.ID != target.Account.ID:
		return ErrImportMergeAccount
	case !mergeableKind(target.Kind) || imported.Kind.IsIncoming() != target.Kind.IsIncoming():
		return ErrImportMergeKind
	case target.FulfilledAt.IsZero() || target.BulkLock() != TxBulkLockNone || target.IsTransferLeg():
		return ErrImportMergeLocked
	}
	return nil
}

// MergeImported is target taking what the statement knows best from
// imported: the amount the account moved, its fee and what it was paid in;
// the rest (day, counterparty, category, notes, past data or not) stays
// target's, but what target lacks and imported has (a description, a
// counterparty, a category of its kind, a note) is taken, and imported's
// tags are added. Lines stay target's (nil).
func MergeImported(imported, target *Transaction) *Transaction {
	merged := *target
	merged.Items, merged.Tags, merged.ReceiptID = nil, nil, ""
	if strings.TrimSpace(merged.Description) == "" {
		merged.Description = imported.Description
	}
	if strings.TrimSpace(merged.Note) == "" {
		merged.Note = imported.Note
	}
	if (merged.Counterparty == nil || merged.Counterparty.ID == "") && imported.Counterparty != nil && imported.Counterparty.ID != "" {
		cp := *imported.Counterparty
		merged.Counterparty = &cp
	}
	// Its category fits target when both take the same kind of category
	// (an expense's fits a refund).
	if (merged.Category == nil || merged.Category.ID == "") && imported.Category != nil && imported.Category.ID != "" &&
		imported.Kind.CategoryKind() == target.Kind.CategoryKind() {
		c := *imported.Category
		merged.Category = &c
	}
	if tags := mergeTags(target.Tags, imported.Tags); tags != nil {
		merged.Tags = tags
	}
	merged.Amount = imported.Amount
	merged.FxFee, merged.Original = 0, nil
	if target.Kind.TakesFxFee() && imported.FxFee > 0 && imported.FxFee < imported.Amount {
		merged.FxFee = imported.FxFee
	}
	if target.Kind.CanBeForeign() && imported.IsForeign() {
		o := *imported.Original
		merged.Original = &o
	}
	return &merged
}

// ImportMergeLikely tells whether target looks like the imported row: within
// 3 days, the same amount, or its amount before the row's fee, or within 3%.
func ImportMergeLikely(imported, target *Transaction) bool {
	days := math.Abs(imported.ScheduledTo.Sub(target.ScheduledTo).Hours() / 24)
	if days > 3.5 {
		return false
	}
	a, b := math.Abs(imported.Amount), math.Abs(target.Amount)
	return RoundCents(a) == RoundCents(b) || RoundCents(a-imported.FxFee) == RoundCents(b) ||
		(a > 0 && math.Abs(a-b)/a <= 0.03)
}

// mergeTags is target's tags plus the ones of imported it lacks (by id, else
// name in any case), nil when imported adds none (leaving target's alone).
func mergeTags(target, imported []Tag) []Tag {
	var add []Tag
	for _, t := range imported {
		if !slices.ContainsFunc(target, func(o Tag) bool {
			return (t.ID != "" && o.ID == t.ID) || strings.EqualFold(o.Name, t.Name)
		}) {
			add = append(add, t)
		}
	}
	if len(add) == 0 {
		return nil
	}
	return append(slices.Clone(target), add...)
}
