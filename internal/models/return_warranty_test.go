package models

import (
	"testing"
	"time"
)

func dayP(s string) *time.Time {
	d := day(s)
	return &d
}

func TestReturnWindow(t *testing.T) {
	today := day("2026-11-23")
	for _, tc := range []struct {
		name       string
		tx         Transaction
		open, soon bool
	}{
		{"none", Transaction{}, false, false},
		{"far", Transaction{ReturnUntil: dayP("2026-12-10")}, true, false},
		{"in 3 days", Transaction{ReturnUntil: dayP("2026-11-26")}, true, true},
		{"last day", Transaction{ReturnUntil: dayP("2026-11-23")}, true, true},
		{"closed", Transaction{ReturnUntil: dayP("2026-11-22")}, false, false},
		{"returned", Transaction{ReturnUntil: dayP("2026-12-10"), ReturnedAt: dayP("2026-11-20")}, false, false},
	} {
		if got := tc.tx.ReturnOpen(today); got != tc.open {
			t.Errorf("%s: open = %t", tc.name, got)
		}
		if got := tc.tx.ReturnSoon(today); got != tc.soon {
			t.Errorf("%s: soon = %t", tc.name, got)
		}
	}
}

func TestWarranty(t *testing.T) {
	today := day("2026-11-23")
	tx := &Transaction{WarrantyUntil: dayP("2027-11-23")}
	own := TxItem{Name: "TV", WarrantyUntil: dayP("2028-11-23")}
	inherited := TxItem{Name: "Cable"}
	charge := TxItem{Kind: TxItemVAT, WarrantyUntil: dayP("2030-01-01")}

	if d, isOwn := own.EffectiveWarranty(tx); !isOwn || !d.Equal(day("2028-11-23")) {
		t.Errorf("own = %v %t", d, isOwn)
	}
	if d, isOwn := inherited.EffectiveWarranty(tx); isOwn || !d.Equal(day("2027-11-23")) {
		t.Errorf("inherited = %v %t", d, isOwn)
	}
	if d, _ := charge.EffectiveWarranty(tx); d != nil {
		t.Errorf("a charge has a warranty: %v", d)
	}

	tx.Items = []TxItem{own, inherited}
	if d := tx.LatestWarranty(); !d.Equal(day("2028-11-23")) {
		t.Errorf("latest = %v", d)
	}
	listed := &Transaction{LinesWarranty: dayP("2027-01-01")}
	if d := listed.LatestWarranty(); d == nil || !d.Equal(day("2027-01-01")) {
		t.Errorf("latest from the list's = %v", d)
	}

	if !WarrantyActive(dayP("2026-11-23"), today) || WarrantyActive(dayP("2026-11-22"), today) || WarrantyActive(nil, today) {
		t.Error("active")
	}
	if !WarrantySoon(dayP("2026-12-23"), today) || WarrantySoon(dayP("2026-12-24"), today) {
		t.Error("soon")
	}
}

func TestParseWindowValue(t *testing.T) {
	purchase := time.Date(2028, 2, 29, 15, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		in, want string
		err      bool
	}{
		{"", "", false},
		{"14d", "2028-03-14", false},
		{"12m", "2029-02-28", false},
		{"24m", "2030-02-28", false},
		{"2028-04-01", "2028-04-01", false},
		{"0d", "", true},
		{"xm", "", true},
		{"2028-13-01", "", true},
	} {
		got, err := ParseWindowValue(tc.in, purchase)
		if (err != nil) != tc.err {
			t.Errorf("%q: err = %v", tc.in, err)
			continue
		}
		if s := formatDayOrEmpty(got); s != tc.want {
			t.Errorf("%q = %q, want %q", tc.in, s, tc.want)
		}
	}
}

func formatDayOrEmpty(d *time.Time) string {
	if d == nil {
		return ""
	}
	return d.Format(time.DateOnly)
}

func TestReturnUntilFor(t *testing.T) {
	d := day("2026-11-23")
	if got := ReturnUntilFor(d, &Counterparty{IsMerchant: true, ReturnDays: 14}); got == nil || !got.Equal(day("2026-12-07")) {
		t.Errorf("merchant = %v", got)
	}
	if got := ReturnUntilFor(d, &Counterparty{ReturnDays: 14}); got != nil {
		t.Errorf("not a merchant = %v", got)
	}
	if got := ReturnUntilFor(d, &Counterparty{IsMerchant: true}); got != nil {
		t.Errorf("no days = %v", got)
	}
	ret, war := WindowsFromReceipt(d, 30, 24)
	if !ret.Equal(day("2026-12-23")) || !war.Equal(day("2028-11-23")) {
		t.Errorf("receipt = %v %v", ret, war)
	}
	if ret, war := WindowsFromReceipt(d, 0, 0); ret != nil || war != nil {
		t.Errorf("none printed = %v %v", ret, war)
	}
}

func TestTxItemsWarrantyCopies(t *testing.T) {
	items := []TxItem{{Name: "TV", Total: 100, WarrantyUntil: dayP("2028-01-01")}, {Kind: TxItemVAT, Total: 14, WarrantyUntil: dayP("2028-01-01")}}
	NormalizeTxItems(items, nil)
	if items[0].WarrantyUntil == nil || items[1].WarrantyUntil != nil {
		t.Errorf("normalized: %v %v", items[0].WarrantyUntil, items[1].WarrantyUntil)
	}
	if cp := CopyTxItems(items); cp[0].WarrantyUntil != nil {
		t.Error("a copy keeps the warranty")
	}
}

func TestTxFilterWindow(t *testing.T) {
	f := ParseTxFilter(map[string][]string{"window": {"return_open"}})
	if f.Window != TxWindowReturnOpen || f.Values().Get("window") != "return_open" || f.ActiveCount() != 1 {
		t.Errorf("filter = %+v", f)
	}
	if f := ParseTxFilter(map[string][]string{"window": {"bogus"}}); f.Window != TxWindowAny {
		t.Errorf("bogus kept: %q", f.Window)
	}
}
