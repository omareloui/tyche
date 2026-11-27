package statementocr

import (
	"math"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/csvimport"
)

func ptr(v float64) *float64 { return &v }

func TestToCSVRoundTrip(t *testing.T) {
	r := &Result{Rows: []Row{
		{Date: "2026-10-01", Description: "IPN Outward\nPayee Name: سيف الدين, \"Living\"", Debit: 85.5, Balance: ptr(7724.51)},
		{Date: "2026-10-04", Time: "13:05", Description: "IPN Inward", Credit: 70000},
	}}
	content, err := r.ToCSV()
	if err != nil {
		t.Fatal(err)
	}
	m := PresetMapping()
	recs, _, err := csvimport.Read(content, m.Delimiter)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(m.Columns(recs)); err != nil {
		t.Fatal(err)
	}
	rows := csvimport.ParseRows(recs, &m, "acc", time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC))
	if len(rows) != 2 {
		t.Fatalf("rows = %d", len(rows))
	}
	if rows[0].Err != "" || rows[0].Amount != -85.5 || rows[0].Description != "IPN Outward Payee Name: سيف الدين, \"Living\"" {
		t.Errorf("row 1 = %+v", rows[0])
	}
	if rows[1].Err != "" || rows[1].Amount != 70000 || !rows[1].HasTime || rows[1].Date.Day() != 4 {
		t.Errorf("row 2 = %+v", rows[1])
	}
}

func TestCheck(t *testing.T) {
	acc := &Result{Kind: KindAccount, OpeningBalance: ptr(100), ClosingBalance: ptr(150),
		Rows: []Row{{Credit: 80}, {Debit: 30}}}
	if ok, diff := acc.Check(); !ok || diff != 0 {
		t.Errorf("account: ok %v, diff %v", ok, diff)
	}
	// a card's balance is owed: purchases add to it
	card := &Result{Kind: KindCard, OpeningBalance: ptr(100), ClosingBalance: ptr(150),
		Rows: []Row{{Debit: 80}, {Credit: 30}}}
	if ok, _ := card.Check(); !ok {
		t.Error("card statement adding up warned")
	}
	card.Rows = card.Rows[:1]
	if ok, diff := card.Check(); ok || diff != -30 {
		t.Errorf("missing row: ok %v, diff %v", ok, diff)
	}
	if ok, _ := (&Result{Rows: []Row{{Debit: 1}}}).Check(); !ok {
		t.Error("no balances printed warned")
	}
}

func TestInspect(t *testing.T) {
	if err := Inspect([]byte("date,amount\n")); err != ErrNotPDF {
		t.Errorf("csv: %v", err)
	}
	if err := Inspect([]byte("%PDF-1.7\ntrailer << /Encrypt 3 0 R >>")); err != ErrPasswordProtected {
		t.Errorf("encrypted: %v", err)
	}
	many := []byte("%PDF-1.4\n")
	for range MaxPages + 1 {
		many = append(many, []byte("<< /Type /Page /Parent 2 0 R >>\n")...)
	}
	if err := Inspect(many); err != ErrTooManyPages {
		t.Errorf("too many pages: %v", err)
	}
	if err := Inspect([]byte("%PDF-1.7\n<< /Type /Pages /Count 4 >>\n<< /Type /Page >>")); err != nil {
		t.Errorf("a plain PDF: %v", err)
	}
}

// The lines of a CIB card statement: fees grouped at the end, 3% of their
// purchase (the ones billed in EGP by foreign merchants too), reversed on
// refunds; Steam's purchase in USD has none.
func cibCard() *Result {
	return &Result{Kind: KindCard, Rows: []Row{
		{Date: "2026-09-15", Description: "Disney Plus London", Debit: 170},
		{Date: "2026-09-19", Description: "WL STEAM PURCHASE BELLEVUE", Debit: 341.49, OriginalAmount: 6.36, OriginalCurrency: "USD"},
		{Date: "2026-09-19", Description: "Google G1SK002M Mountain View", Debit: 94.99},
		{Date: "2026-09-28", Description: "aliexpress Luxembourg", Debit: 1728.29},
		{Date: "2026-09-28", Description: "aliexpress Luxembourg", Debit: 1458.78},
		{Date: "2026-09-28", Description: "aliexpress Luxembourg", Credit: 917.95},
		{Date: "2026-10-04", Description: "PAYMENT RECEIVED INTERNET", Credit: 40000},
		{Date: "2026-09-15", Description: "FOREIGN EXCHANGE FEES", Debit: 5.10, IsFxFee: true},
		{Date: "2026-09-19", Description: "FOREIGN EXCHANGE FEES", Debit: 2.84, IsFxFee: true},
		{Date: "2026-09-28", Description: "FOREIGN EXCHANGE FEES", Debit: 51.84, IsFxFee: true},
		{Date: "2026-09-28", Description: "FOREIGN EXCHANGE FEES", Debit: 43.76, IsFxFee: true},
		{Date: "2026-09-28", Description: "FOREIGN EXCHANGE FEES REV", Credit: 27.53, IsFxFee: true},
		{Date: "2026-10-09", Description: "FOREIGN EXCHANGE FEES", Debit: 9.99, IsFxFee: true},
	}}
}

func TestMergeFxFees(t *testing.T) {
	for _, pct := range []float64{3, 0} {
		r := cibCard()
		var before float64
		for _, row := range r.Rows {
			before += row.Debit - row.Credit
		}
		m := r.MergeFxFees(pct)
		if m.Merged != 5 || m.Unmatched != 1 || m.Percent != 3 || m.Inferred != (pct == 0) {
			t.Errorf("pct %v: merge = %+v", pct, m)
		}
		var after float64
		for _, row := range r.Rows {
			after += row.Debit - row.Credit
		}
		if math.Abs(after-before) > 1e-9 {
			t.Errorf("pct %v: sums %v → %v", pct, before, after)
		}
		want := map[string]Row{
			"Disney Plus London":            {Debit: 175.10, FxFee: 5.10},
			"WL STEAM PURCHASE BELLEVUE":    {Debit: 341.49},
			"Google G1SK002M Mountain View": {Debit: 97.83, FxFee: 2.84},
		}
		for _, row := range r.Rows {
			if w, ok := want[row.Description]; ok && (row.Debit != w.Debit || row.FxFee != w.FxFee) {
				t.Errorf("pct %v: %s = %v (fee %v), want %v (fee %v)", pct, row.Description, row.Debit, row.FxFee, w.Debit, w.FxFee)
			}
		}
		if got := r.Rows[3].Debit; got != 1780.13 {
			t.Errorf("pct %v: aliexpress 1728.29 = %v, want 1780.13", pct, got)
		}
		if got := r.Rows[5].Credit; got != 945.48 {
			t.Errorf("pct %v: refund = %v, want 945.48 with its fee back", pct, got)
		}
		if len(r.Rows) != 8 || !r.Rows[7].IsFxFee {
			t.Errorf("pct %v: %d rows left, the unmatched fee last: %+v", pct, len(r.Rows), r.Rows[len(r.Rows)-1])
		}
	}
}

func TestToCSVForeignColumns(t *testing.T) {
	r := cibCard()
	r.MergeFxFees(3)
	content, err := r.ToCSV()
	if err != nil {
		t.Fatal(err)
	}
	m := PresetMapping()
	recs, _, err := csvimport.Read(content, m.Delimiter)
	if err != nil {
		t.Fatal(err)
	}
	rows := csvimport.ParseRows(recs, &m, "acc", time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC))
	if rows[0].Amount != -175.10 || rows[0].FxFee != 5.10 || rows[0].OriginalCurrency != "" {
		t.Errorf("disney = %+v", rows[0])
	}
	if rows[1].OriginalAmount != 6.36 || rows[1].OriginalCurrency != "USD" || rows[1].FxFee != 0 {
		t.Errorf("steam = %+v", rows[1])
	}
}

func TestPresetMappingForOldFiles(t *testing.T) {
	// a file of the 6 columns ToCSV wrote first
	content := []byte("date,time,description,debit,credit,balance\n2026-10-01,,Uber,85.5,,\n")
	m := PresetMapping()
	recs, _, err := csvimport.Read(content, m.Delimiter)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(m.Columns(recs)); err == nil {
		t.Fatal("the current preset reads a 6 columns file")
	}
	old := PresetMappingFor(m.Columns(recs))
	if err := old.Validate(old.Columns(recs)); err != nil {
		t.Fatal(err)
	}
	if rows := csvimport.ParseRows(recs, &old, "acc", time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)); rows[0].Err != "" || rows[0].Amount != -85.5 {
		t.Errorf("row = %+v", rows[0])
	}
	if CacheKey([]byte("a")) == CacheKey([]byte("b")) {
		t.Error("cache keys collide")
	}
}
