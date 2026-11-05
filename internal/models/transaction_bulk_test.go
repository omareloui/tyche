package models

import (
	"slices"
	"testing"
	"time"
)

func TestTxBulkEditApply(t *testing.T) {
	today := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	food := &Category{ID: "food", Kinds: []TxKind{ExpenseTxKind}}
	shop := &Counterparty{ID: "shop"}
	at := time.Date(2026, 11, 1, 14, 30, 0, 0, time.UTC)
	tx := func(k TxKind, fulfilled bool) *Transaction {
		t := &Transaction{ID: "t", Kind: k, ScheduledTo: at, Category: &Category{ID: "old"}}
		if fulfilled {
			t.FulfilledAt = at
		}
		return t
	}
	day := func(d int) *time.Time {
		t := time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC)
		return &t
	}

	tests := []struct {
		name      string
		edit      TxBulkEdit
		tx        *Transaction
		wantCat   string
		wantCp    string
		wantDay   int
		wantSkips []TxBulkSkip
	}{
		{"category fits", TxBulkEdit{Category: food}, tx(ExpenseTxKind, true), "food", "", 1, nil},
		{"category of a refund", TxBulkEdit{Category: food}, tx(RefundTxKind, true), "food", "", 1, nil},
		{"category doesn't fit", TxBulkEdit{Category: food}, tx(IncomeTxKind, true), "old", "", 1, []TxBulkSkip{TxBulkSkipCategory}},
		{"clear category", TxBulkEdit{ClearCategory: true}, tx(IncomeTxKind, true), "", "", 1, nil},
		{"counterparty", TxBulkEdit{Counterparty: shop}, tx(ExpenseTxKind, true), "old", "shop", 1, nil},
		{"transfer takes none", TxBulkEdit{Counterparty: shop, Category: food}, tx(TransferTxKind, true), "old", "", 1, []TxBulkSkip{TxBulkSkipCategory, TxBulkSkipCounterparty}},
		{"move back", TxBulkEdit{Date: day(3)}, tx(ExpenseTxKind, true), "old", "", 3, nil},
		{"fulfilled today", TxBulkEdit{Date: day(5)}, tx(ExpenseTxKind, true), "old", "", 5, nil},
		{"fulfilled ahead", TxBulkEdit{Date: day(6)}, tx(ExpenseTxKind, true), "old", "", 1, []TxBulkSkip{TxBulkSkipAhead}},
		{"scheduled ahead", TxBulkEdit{Date: day(20)}, tx(ExpenseTxKind, false), "old", "", 20, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, skips := tt.edit.Apply(tt.tx, today)
			if !slices.Equal(skips, tt.wantSkips) {
				t.Fatalf("skips = %v, want %v", skips, tt.wantSkips)
			}
			cat, cp := "", ""
			if got.Category != nil {
				cat = got.Category.ID
			}
			if got.Counterparty != nil {
				cp = got.Counterparty.ID
			}
			if cat != tt.wantCat || cp != tt.wantCp {
				t.Errorf("category, counterparty = %q, %q, want %q, %q", cat, cp, tt.wantCat, tt.wantCp)
			}
			if got.ScheduledTo.Day() != tt.wantDay || got.ScheduledTo.Hour() != 14 || got.ScheduledTo.Minute() != 30 {
				t.Errorf("scheduled to %v, want day %d at 14:30", got.ScheduledTo, tt.wantDay)
			}
			if tt.tx.Category.ID != "old" {
				t.Errorf("changed the original")
			}
		})
	}

	locked := tx(ExpenseTxKind, true)
	locked.Account.DeletedAt = at
	if got, skips := (&TxBulkEdit{Category: food}).Apply(locked, today); got != nil || !slices.Equal(skips, []TxBulkSkip{TxBulkSkipLocked}) {
		t.Errorf("locked: got %v, %v", got, skips)
	}
	if got, _ := (&TxBulkEdit{Category: food}).Apply(tx(AdjustmentTxKind, true), today); got != nil {
		t.Errorf("adjustment edited")
	}
}

func TestTxFilterNone(t *testing.T) {
	f := ParseTxFilter(map[string][]string{TxFilterCategory: {TxFilterNone}})
	if f.CategoryID != TxFilterNone || f.Values().Get(TxFilterCategory) != TxFilterNone {
		t.Errorf("category = %q", f.CategoryID)
	}
}
