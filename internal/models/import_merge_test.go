package models

import "testing"

func TestMergeImportedFillsBlanks(t *testing.T) {
	imported := &Transaction{
		Kind: ExpenseTxKind, Amount: 120, Description: "UBER TRIP", Note: "from the bank",
		Counterparty: &Counterparty{ID: "cp1"}, Category: &Category{ID: "cat1"},
		Tags: []Tag{{ID: "t1", Name: "Trip"}, {ID: "t2", Name: "Work"}},
	}

	t.Run("blank target takes them", func(t *testing.T) {
		got := MergeImported(imported, &Transaction{Kind: ExpenseTxKind, Amount: 100})
		if got.Amount != 120 || got.Description != "UBER TRIP" || got.Note != "from the bank" {
			t.Fatalf("got %+v", got)
		}
		if got.Counterparty == nil || got.Counterparty.ID != "cp1" || got.Category == nil || got.Category.ID != "cat1" {
			t.Fatalf("refs not taken: %+v", got)
		}
		if len(got.Tags) != 2 {
			t.Fatalf("tags = %v", got.Tags)
		}
		// copies, not imported's own
		got.Counterparty.ID = "x"
		if imported.Counterparty.ID != "cp1" {
			t.Fatal("shared the counterparty")
		}
	})

	t.Run("target's own stay", func(t *testing.T) {
		target := &Transaction{
			Kind: ExpenseTxKind, Description: "Ride home", Note: "mine",
			Counterparty: &Counterparty{ID: "cp2"}, Category: &Category{ID: "cat2"},
			Tags: []Tag{{ID: "t1", Name: "Trip"}},
		}
		got := MergeImported(imported, target)
		if got.Description != "Ride home" || got.Note != "mine" || got.Counterparty.ID != "cp2" || got.Category.ID != "cat2" {
			t.Fatalf("overwrote target's: %+v", got)
		}
		if len(got.Tags) != 2 || got.Tags[0].ID != "t1" || got.Tags[1].ID != "t2" {
			t.Fatalf("tags = %v", got.Tags)
		}
	})

	t.Run("no tags to add leaves them alone", func(t *testing.T) {
		got := MergeImported(&Transaction{Kind: ExpenseTxKind, Tags: []Tag{{Name: "trip"}}}, &Transaction{Kind: ExpenseTxKind, Tags: []Tag{{ID: "t1", Name: "Trip"}}})
		if got.Tags != nil {
			t.Fatalf("tags = %v, want nil", got.Tags)
		}
	})

	t.Run("category of another kind isn't taken", func(t *testing.T) {
		got := MergeImported(&Transaction{Kind: IncomeTxKind, Category: &Category{ID: "salary"}}, &Transaction{Kind: IncomeTxKind})
		if got.Category == nil {
			t.Fatal("same kind should take it")
		}
		got = MergeImported(&Transaction{Kind: ExpenseTxKind, Category: &Category{ID: "food"}}, &Transaction{Kind: RefundTxKind})
		if got.Category == nil || got.Category.ID != "food" {
			t.Fatal("an expense's category fits a refund")
		}
		got = MergeImported(&Transaction{Kind: IncomeTxKind, Category: &Category{ID: "salary"}}, &Transaction{Kind: RefundTxKind})
		if got.Category != nil {
			t.Fatal("an income's category doesn't fit a refund")
		}
	})
}
