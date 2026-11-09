package csvimport

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

var today = time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParseAmount(t *testing.T) {
	tests := []struct {
		in    string
		comma bool
		want  float64
		ok    bool
	}{
		{"1,234.56", false, 1234.56, true},
		{"-45.10", false, -45.1, true},
		{"45.10-", false, -45.1, true},
		{"(1,000.00)", false, -1000, true},
		{"250.00 DR", false, -250, true},
		{"250.00 CR", false, 250, true},
		{"EGP 1,500", false, 1500, true},
		{"1.234,56", true, 1234.56, true},
		{"-12,5", true, -12.5, true},
		{"", false, 0, false},
		{"n/a", false, 0, false},
	}
	for _, tt := range tests {
		got, ok := ParseAmount(tt.in, tt.comma)
		if ok != tt.ok || got != tt.want {
			t.Errorf("ParseAmount(%q, %v) = %v, %v, want %v, %v", tt.in, tt.comma, got, ok, tt.want, tt.ok)
		}
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		in, format string
		want       time.Time
		clock      time.Duration
	}{
		{"05/11/2026", "DD/MM/YYYY", day("2026-11-05"), 0},
		{"2026-11-05", "YYYY-MM-DD", day("2026-11-05"), 0},
		{"2026-11-05T13:45:10Z", "YYYY-MM-DD", day("2026-11-05"), 13*time.Hour + 45*time.Minute + 10*time.Second},
		{"05 Nov 2026 9:05 PM", "DD MMM YYYY", day("2026-11-05"), 21*time.Hour + 5*time.Minute},
		{"5-nov-26", "DD-MMM-YY", day("2026-11-05"), 0},
	}
	for _, tt := range tests {
		layout, err := smsparser.DateLayout(tt.format)
		if err != nil {
			t.Fatal(err)
		}
		got, clock, ok := ParseDate(tt.in, layout)
		if !ok || !got.Equal(tt.want) || clock != tt.clock {
			t.Errorf("ParseDate(%q) = %v, %v, %v, want %v, %v", tt.in, got, clock, ok, tt.want, tt.clock)
		}
	}
}

func TestGuessDateFormat(t *testing.T) {
	tests := []struct {
		vals []string
		want string
	}{
		{[]string{"05/11/2026", "28/10/2026"}, "DD/MM/YYYY"},
		{[]string{"11/05/2026", "10/28/2026"}, "MM/DD/YYYY"},
		{[]string{"2026-11-05 10:00:00"}, "YYYY-MM-DD"},
		{[]string{"05 Nov 2026"}, "DD MMM YYYY"},
		{[]string{"yesterday"}, ""},
	}
	for _, tt := range tests {
		if got := GuessDateFormat(tt.vals); got != tt.want {
			t.Errorf("GuessDateFormat(%q) = %q, want %q", tt.vals, got, tt.want)
		}
	}
}

func TestGuessWithHeader(t *testing.T) {
	data := "\xef\xbb\xbfAccount statement\nAccount;1234\n\nPosting Date;Description;Debit;Credit;Balance\n" +
		"05/11/2026;POS CARREFOUR MAADI;1.250,50;;10.000,00\n" +
		"06/11/2026;SALARY;;20.000,00;30.000,00\n"
	recs, delim, err := Read([]byte(data), "")
	if err != nil {
		t.Fatal(err)
	}
	if delim != ";" {
		t.Fatalf("delimiter = %q, want ;", delim)
	}
	m := Guess(recs, delim)
	if !m.HasHeader || m.SkipRows != 2 || m.Date != 0 || m.Description != 1 || !m.Split || m.Debit != 2 || m.Credit != 3 ||
		m.Amount != None || !m.DecimalComma || m.DateFormat != "DD/MM/YYYY" {
		t.Fatalf("Guess = %+v", m)
	}
	if err := m.Validate(m.Columns(recs)); err != nil {
		t.Fatal(err)
	}

	rows := ParseRows(recs, &m, "acc", today)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if r := rows[0]; r.Err != "" || r.Amount != -1250.5 || !r.Date.Equal(day("2026-11-05")) || r.Description != "POS CARREFOUR MAADI" || r.Line != 4 {
		t.Errorf("row 0 = %+v", r)
	}
	if r := rows[1]; r.Err != "" || r.Amount != 20000 {
		t.Errorf("row 1 = %+v", r)
	}
}

func TestGuessWithoutHeader(t *testing.T) {
	data := "2026-11-01,Uber trip,-85.50\n2026-11-02,Refund,20\n"
	recs, delim, err := Read([]byte(data), "")
	if err != nil {
		t.Fatal(err)
	}
	m := Guess(recs, delim)
	if m.HasHeader || m.SkipRows != 0 || m.Date != 0 || m.Amount != 2 || m.Description != 1 || m.Split || m.DateFormat != "YYYY-MM-DD" {
		t.Fatalf("Guess = %+v", m)
	}
	rows := ParseRows(recs, &m, "acc", today)
	if len(rows) != 2 || rows[0].Amount != -85.5 || rows[1].Amount != 20 || rows[0].Line != 1 {
		t.Fatalf("rows = %+v", rows)
	}

	m.Negative = NegativeIn
	if rows := ParseRows(recs, &m, "acc", today); rows[0].Amount != 85.5 {
		t.Errorf("negative in: amount = %v, want 85.5", rows[0].Amount)
	}
}

func TestGuessMerchantColumn(t *testing.T) {
	recs := [][]string{
		{"Date", "Merchant", "Currency", "Amount"},
		{"2026-11-01T10:15:00", "AMAZON.EG", "EGP", "899.00"},
	}
	m := Guess(recs, ",")
	if m.Counterparty != 1 || m.Currency != 2 || m.Description != None {
		t.Fatalf("Guess = %+v", m)
	}
	rows := ParseRows(recs, &m, "acc", today)
	if r := rows[0]; r.Counterparty != "AMAZON.EG" || r.Description != "" || r.Clock != 10*time.Hour+15*time.Minute || !r.HasTime {
		t.Fatalf("row = %+v", r)
	}
}

func TestReadUTF16(t *testing.T) {
	text := "Date\tAmount\n01/11/2026\t-10\n"
	b := []byte{0xFF, 0xFE}
	for _, r := range text {
		b = append(b, byte(r), 0)
	}
	recs, delim, err := Read(b, "")
	if err != nil {
		t.Fatal(err)
	}
	if delim != "\t" || len(recs) != 2 || recs[1][1] != "-10" {
		t.Fatalf("Read = %q, %q", recs, delim)
	}
}

func TestParseRowsErrors(t *testing.T) {
	recs := [][]string{
		{"Date", "Amount", "Description"},
		{"01/11/2026", "-10", "ok"},
		{"", "-10", "no date"},
		{"31/02/2026", "-10", "bad date"},
		{"01/12/2026", "-10", "ahead"},
		{"02/11/2026", "", "no amount"},
		{"02/11/2026", "0", "zero"},
	}
	m := Guess(recs, ",")
	rows := ParseRows(recs, &m, "acc", today)
	want := []string{"", "No date", "Date not in the format DD/MM/YYYY", "Dated ahead", "No amount", "Zero amount"}
	for i, w := range want {
		if rows[i].Err != w {
			t.Errorf("row %d err = %q, want %q", i, rows[i].Err, w)
		}
		if (rows[i].Hash == "") != (w != "") {
			t.Errorf("row %d hash = %q", i, rows[i].Hash)
		}
	}
}

func TestRowHashes(t *testing.T) {
	recs := [][]string{
		{"Date", "Amount", "Description"},
		{"01/11/2026", "-40", "Coffee"},
		{"01/11/2026", "-40", "Coffee"},
	}
	m := Guess(recs, ",")
	a := ParseRows(recs, &m, "acc", today)
	if a[0].Hash == a[1].Hash {
		t.Fatal("same rows got the same hash")
	}
	b := ParseRows(recs, &m, "acc", today)
	if a[0].Hash != b[0].Hash || a[1].Hash != b[1].Hash {
		t.Fatal("hashes aren't stable")
	}
	if other := ParseRows(recs, &m, "other", today); other[0].Hash == a[0].Hash {
		t.Fatal("another account's row got the same hash")
	}
}

func TestMatchDuplicates(t *testing.T) {
	egp := &models.Currency{String: "EGP"}
	rows := []Row{
		{Line: 1, Date: day("2026-11-01"), Amount: -100, Hash: "h1"},
		{Line: 2, Date: day("2026-11-02"), Amount: -50, Hash: "h2"},
		{Line: 3, Date: day("2026-11-02"), Amount: -50, Hash: "h3"},
		{Line: 4, Date: day("2026-11-03"), Amount: 50, Hash: "h4"},
		{Line: 5, Date: day("2026-11-05"), Amount: -70, Hash: "h5"},
		{Line: 6, Err: "No date"},
	}
	existing := []Existing{
		{ID: "imported", Day: day("2026-11-01"), Out: true, Moved: 100, Hash: "h1"},
		// an SMS the day after: one of the two same rows
		{ID: "sms", Day: day("2026-11-03"), Out: true, Moved: 50},
		// two days apart: not it
		{ID: "far", Day: day("2026-11-07"), Out: true, Moved: 70},
	}
	got := MatchDuplicates(rows, existing, egp)
	want := []Match{
		{Status: RowImported, TxID: "imported"},
		{Status: RowDuplicate, TxID: "sms"},
		{Status: RowNew},
		{Status: RowNew},
		{Status: RowNew},
		{Status: RowError},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestFindCounterparty(t *testing.T) {
	cps := []models.Counterparty{
		{ID: "1", Name: "Uber"},
		{ID: "2", Name: "Uber Eats"},
		{ID: "3", Name: "Carrefour", Aliases: []string{"CRF MAADI"}},
		{ID: "4", Name: "Al"},
	}
	tests := []struct{ text, want string }{
		{"POS UBER EATS CAIRO", "2"},
		{"UBER *TRIP", "1"},
		{"PURCHASE CRF MAADI 1234", "3"},
		{"SUBERB", ""},
		{"AL AHLY", ""},
	}
	for _, tt := range tests {
		got := FindCounterparty(tt.text, cps)
		if (got == nil && tt.want != "") || (got != nil && got.ID != tt.want) {
			t.Errorf("FindCounterparty(%q) = %v, want %q", tt.text, got, tt.want)
		}
	}
}
