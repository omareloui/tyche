package models

import "testing"

func ptr(v float64) *float64 { return &v }

func TestNormalizeTxItems(t *testing.T) {
	items := []TxItem{
		{Name: "Coffee", Quantity: 2, UnitPrice: 45},
		{Name: "Croissant", Total: 60},
		{Name: "Water", Quantity: 3, Total: 30},
		{Kind: TxItemVAT, Rate: ptr(14), Included: true},
		{Kind: TxItemService, Rate: ptr(12)},
		{Kind: TxItemDiscount, Total: 15},
		{Kind: TxItemTip, Total: 10.005},
	}
	NormalizeTxItems(items, nil)

	want := []struct{ qty, unit, total float64 }{
		{2, 45, 90},
		{1, 60, 60},
		{3, 10, 30},
		{1, 25.2, 25.2},   // 14% of 180
		{1, 21.6, 21.6},   // 12% of 180
		{1, -15, -15},     // discounts subtract
		{1, 10.01, 10.01}, // rounded to cents
	}
	for i, w := range want {
		it := items[i]
		if it.Quantity != w.qty || it.UnitPrice != w.unit || it.Total != w.total {
			t.Errorf("item %d = qty %v unit %v total %v, want %v %v %v", i, it.Quantity, it.UnitPrice, it.Total, w.qty, w.unit, w.total)
		}
	}

	if got := TxItemsSubtotal(items); got != 180 {
		t.Errorf("subtotal = %v, want 180", got)
	}
	// The included VAT isn't added.
	if got, want := TxItemsTotal(items), 180+21.6-15+10.01; got-want > 1e-9 || want-got > 1e-9 {
		t.Errorf("total = %v, want %v", got, want)
	}
	if TxItemsMismatch(items, -(180 + 21.6 - 15 + 10.01)) {
		t.Error("expected no mismatch with the amount the lines add up to (sign ignored)")
	}
	if !TxItemsMismatch(items, 200) {
		t.Error("expected a mismatch")
	}
	if TxItemsMismatch(nil, 200) {
		t.Error("no lines, no mismatch")
	}
}

func TestNormalizeTxItemsMetal(t *testing.T) {
	gold := &Currency{String: "XAU"}
	items := []TxItem{{Name: "Ring", Quantity: 1, UnitPrice: 0.1234567}}
	NormalizeTxItems(items, gold)
	if items[0].Total != 0.123457 {
		t.Errorf("total = %v, want 0.123457", items[0].Total)
	}
}

func TestTxItemLabel(t *testing.T) {
	cases := []struct {
		it   TxItem
		want string
	}{
		{TxItem{Name: "Milk"}, "Milk"},
		{TxItem{Kind: TxItemVAT, Rate: ptr(14)}, "VAT 14%"},
		{TxItem{Kind: TxItemService}, "Service"},
		{TxItem{Kind: TxItemVAT, Name: "ض.ق.م"}, "ض.ق.م"},
	}
	for _, c := range cases {
		if got := c.it.Label(); got != c.want {
			t.Errorf("Label() = %q, want %q", got, c.want)
		}
	}
}

func TestCopyAndSortTxItems(t *testing.T) {
	items := []TxItem{{ID: "a", Kind: TxItemVAT, Rate: ptr(14)}, {ID: "b", Name: "Tea"}}
	cp := CopyTxItems(items)
	if cp[0].ID == "a" || cp[1].ID == "b" || cp[0].ID == "" {
		t.Errorf("expected new IDs, got %q %q", cp[0].ID, cp[1].ID)
	}
	*cp[0].Rate = 5
	if *items[0].Rate != 14 {
		t.Error("the copy shares the rate")
	}
	sorted := SortTxItems(items)
	if sorted[0].ID != "b" || sorted[1].ID != "a" {
		t.Errorf("expected items before charges, got %q %q", sorted[0].ID, sorted[1].ID)
	}
	if CopyTxItems(nil) != nil {
		t.Error("expected nil")
	}
}

func TestFormatBytes(t *testing.T) {
	for n, want := range map[int64]string{0: "", 300: "1 KB", 186_000: "182 KB", 2_500_000: "2.4 MB"} {
		if got := FormatBytes(n); got != want {
			t.Errorf("FormatBytes(%d) = %q, want %q", n, got, want)
		}
	}
}
