// Package csvimport reads bank CSV exports into transactions.
package csvimport

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	xunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

// Bounds of a CSV import.
const (
	MaxBytes = 5 << 20
	MaxRows  = 5000
)

// None is a Mapping column not in the file.
const None = -1

// Sign tells which way a signed amount column goes.
type Sign string

const (
	// NegativeOut: negative amounts left the account (most banks).
	NegativeOut Sign = "out"
	// NegativeIn: negative amounts came in (e.g. some cards' statements,
	// where purchases are positive).
	NegativeIn Sign = "in"
)

// Delimiters are the separators a CSV file may use.
var Delimiters = []string{",", ";", "\t", "|"}

// DateFormats are the date formats offered (and guessed, in this order:
// days first, as in Egyptian banks).
var DateFormats = []string{
	"DD/MM/YYYY", "DD-MM-YYYY", "DD.MM.YYYY", "YYYY-MM-DD", "YYYY/MM/DD",
	"DD/MM/YY", "DD-MM-YY", "DD MMM YYYY", "DD-MMM-YYYY", "DD MMM YY", "DD-MMM-YY",
	"MM/DD/YYYY", "MM-DD-YYYY", "MM/DD/YY", "MMM DD YYYY", "MMM DD, YYYY",
}

// Mapping is how a bank's CSV export reads: its columns (0-based, None
// when missing) and formats.
type Mapping struct {
	Delimiter string `json:"delimiter"`
	// SkipRows are the rows before the header (or the first transaction when
	// there's no header), e.g. a statement's account details.
	SkipRows   int    `json:"skip_rows"`
	HasHeader  bool   `json:"has_header"`
	Date       int    `json:"date"`
	DateFormat string `json:"date_format"`
	Time       int    `json:"time"`
	// Amount is a signed amount, else (Split) Debit (out) and Credit (in).
	Split        bool `json:"split"`
	Amount       int  `json:"amount"`
	Debit        int  `json:"debit"`
	Credit       int  `json:"credit"`
	Negative     Sign `json:"negative"`
	Description  int  `json:"description"`
	Counterparty int  `json:"counterparty"`
	Currency     int  `json:"currency"`
	// DecimalComma reads 1.234,56 instead of 1,234.56.
	DecimalComma bool `json:"decimal_comma"`
}

// NewMapping is a mapping without any column.
func NewMapping() Mapping {
	return Mapping{
		Delimiter: ",", Date: None, Time: None, Amount: None, Debit: None, Credit: None,
		Negative: NegativeOut, Description: None, Counterparty: None, Currency: None,
		DateFormat: DateFormats[0],
	}
}

// Validate tells what the mapping misses to read transactions.
func (m *Mapping) Validate(columns int) error {
	if !slices.Contains(Delimiters, m.Delimiter) {
		return errors.New("Pick the separator.")
	}
	if m.SkipRows < 0 {
		return errors.New("The rows to skip can't be negative.")
	}
	for _, c := range []int{m.Date, m.Time, m.Amount, m.Debit, m.Credit, m.Description, m.Counterparty, m.Currency} {
		if c < None || c >= columns {
			return errors.New("A picked column isn't in the file.")
		}
	}
	if m.Date == None {
		return errors.New("Pick the date's column.")
	}
	if m.Split && m.Debit == None && m.Credit == None {
		return errors.New("Pick the debit and credit columns.")
	}
	if !m.Split && m.Amount == None {
		return errors.New("Pick the amount's column.")
	}
	if _, err := smsparser.DateLayout(m.DateFormat); err != nil {
		return errors.New("Pick a valid date format.")
	}
	if m.Negative != NegativeOut && m.Negative != NegativeIn {
		m.Negative = NegativeOut
	}
	return nil
}

// Read decodes a CSV file (UTF-8 with or without a BOM, or UTF-16 with
// one) into its records, with delim or the separator it seems to use when
// empty, returned along with them.
func Read(data []byte, delim string) ([][]string, string, error) {
	text, err := decodeCSVText(data)
	if err != nil {
		return nil, "", err
	}
	if delim == "" {
		delim = sniffCSVDelimiter(text)
	}
	recs, err := parseCSV(text, delim, MaxRows+100)
	if err != nil {
		return nil, delim, err
	}
	if len(recs) == 0 {
		return nil, delim, errors.New("The file has no rows.")
	}
	return recs, delim, nil
}

func decodeCSVText(data []byte) (string, error) {
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}), bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		dec := xunicode.UTF16(xunicode.LittleEndian, xunicode.UseBOM).NewDecoder()
		out, _, err := transform.Bytes(dec, data)
		if err != nil {
			return "", errors.New("The file's text can't be read.")
		}
		data = out
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if bytes.IndexByte(data, 0) >= 0 {
		return "", errors.New("This isn't a CSV file.")
	}
	if !utf8.Valid(data) {
		return strings.ToValidUTF8(string(data), "\uFFFD"), nil
	}
	return string(data), nil
}

func parseCSV(text, delim string, maxRows int) ([][]string, error) {
	r := csv.NewReader(strings.NewReader(text))
	r.Comma, _ = utf8.DecodeRuneInString(delim)
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	r.ReuseRecord = false
	var recs [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("The file isn't a valid CSV.")
		}
		if slices.IndexFunc(rec, func(s string) bool { return strings.TrimSpace(s) != "" }) < 0 {
			continue
		}
		if len(recs) == maxRows {
			return nil, fmt.Errorf("The file has more than %d rows, split it.", MaxRows)
		}
		recs = append(recs, rec)
	}
	return recs, nil
}

// sniffCSVDelimiter picks the separator giving the most rows of the same
// (more than one) number of fields, in the file's first lines.
func sniffCSVDelimiter(text string) string {
	lines := strings.SplitN(text, "\n", 31)
	if len(lines) > 30 {
		lines = lines[:30]
	}
	head := strings.Join(lines, "\n")
	best, bestRows, bestFields := ",", 0, 0
	for _, d := range Delimiters {
		recs, err := parseCSV(head, d, 100)
		if err != nil {
			continue
		}
		fields, rows := modeFieldCount(recs)
		if fields < 2 {
			continue
		}
		if rows > bestRows || (rows == bestRows && fields > bestFields) {
			best, bestRows, bestFields = d, rows, fields
		}
	}
	return best
}

// modeFieldCount is the most common number of fields and how many rows have it.
func modeFieldCount(recs [][]string) (fields, rows int) {
	counts := map[int]int{}
	for _, r := range recs {
		counts[len(r)]++
	}
	for f, n := range counts {
		if n > rows || (n == rows && f > fields) {
			fields, rows = f, n
		}
	}
	return fields, rows
}

// HeaderSig identifies a file's layout by its header, to pick its saved
// mapping again.
func HeaderSig(header []string) string {
	cols := make([]string, len(header))
	for i, h := range header {
		cols[i] = strings.ToLower(strings.Join(strings.Fields(h), " "))
	}
	return strings.Join(cols, "|")
}

// Header is the mapping's header row, nil without one.
func (m *Mapping) Header(recs [][]string) []string {
	if !m.HasHeader || m.SkipRows >= len(recs) {
		return nil
	}
	return recs[m.SkipRows]
}

// DataRows are the records holding transactions, from the first.
func (m *Mapping) DataRows(recs [][]string) [][]string {
	start := m.SkipRows
	if m.HasHeader {
		start++
	}
	if start >= len(recs) {
		return nil
	}
	return recs[start:]
}

// Columns is how many columns the file has (its header's, else its widest
// row's).
func (m *Mapping) Columns(recs [][]string) int {
	if h := m.Header(recs); h != nil {
		return len(h)
	}
	n := 0
	for _, r := range recs {
		n = max(n, len(r))
	}
	return n
}

var keywords = []struct {
	field string
	words []string
}{
	{"date", []string{"transaction date", "trans date", "txn date", "posting date", "post date", "booking date", "date", "تاريخ"}},
	{"time", []string{"time", "الوقت"}},
	{"debit", []string{"debit", "withdraw", "money out", "paid out", "مدين", "سحب", "خصم"}},
	{"credit", []string{"credit", "deposit", "money in", "paid in", "دائن", "إيداع", "ايداع"}},
	{"amount", []string{"amount", "value", "المبلغ", "مبلغ", "القيمة"}},
	{"description", []string{"description", "details", "narrative", "narration", "particulars", "memo", "remarks", "البيان", "الوصف", "تفاصيل"}},
	{"counterparty", []string{"merchant", "payee", "beneficiary", "counterparty", "التاجر", "المستفيد"}},
	{"currency", []string{"currency", "ccy", "العملة"}},
}

// columnFields is the field each header cell names, "" for none.
func columnFields(header []string) []string {
	fields := make([]string, len(header))
	for i, h := range header {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" || strings.Contains(h, "balance") || strings.Contains(h, "رصيد") {
			continue
		}
		// a "Debit/Credit" indicator column isn't an amount
		if strings.Contains(h, "debit") && strings.Contains(h, "credit") {
			continue
		}
		for _, k := range keywords {
			if slices.ContainsFunc(k.words, func(w string) bool { return strings.Contains(h, w) }) {
				fields[i] = k.field
				break
			}
		}
	}
	return fields
}

// Guess works out how a CSV file reads: the header among its first
// rows, its columns by their names (else by their content) and the formats of
// its dates and amounts.
func Guess(recs [][]string, delim string) Mapping {
	m := NewMapping()
	m.Delimiter = delim

	fields, _ := modeFieldCount(recs)
	headerAt, hits := -1, 1
	for i := 0; i < len(recs) && i < 20; i++ {
		n := 0
		for _, f := range columnFields(recs[i]) {
			if f != "" {
				n++
			}
		}
		if n > hits {
			headerAt, hits = i, n
		}
	}

	if headerAt >= 0 {
		m.SkipRows, m.HasHeader = headerAt, true
		for i, f := range columnFields(recs[headerAt]) {
			col := map[string]*int{
				"date": &m.Date, "time": &m.Time, "debit": &m.Debit, "credit": &m.Credit, "amount": &m.Amount,
				"description": &m.Description, "counterparty": &m.Counterparty, "currency": &m.Currency,
			}[f]
			if col != nil && *col == None {
				*col = i
			}
		}
	} else {
		for i, r := range recs {
			if len(r) == fields {
				m.SkipRows = i
				break
			}
		}
	}

	rows := m.DataRows(recs)
	if len(rows) > 30 {
		rows = rows[:30]
	}
	sample := func(col int) []string {
		var vals []string
		for _, r := range rows {
			if col < len(r) && strings.TrimSpace(r[col]) != "" {
				vals = append(vals, strings.TrimSpace(r[col]))
			}
		}
		return vals
	}

	// without names, by content: the first column of dates, the first one of
	// amounts after it, the longest text
	if m.Date == None {
		for c := range fields {
			if vals := sample(c); len(vals) > 0 && GuessDateFormat(vals) != "" {
				m.Date = c
				break
			}
		}
	}
	if m.Amount == None && m.Debit == None && m.Credit == None {
		for c := range fields {
			if c == m.Date || c == m.Time {
				continue
			}
			if vals := sample(c); len(vals) > 0 && allAmounts(vals) {
				m.Amount = c
				break
			}
		}
	}
	if m.Debit != None || m.Credit != None {
		m.Split, m.Amount = true, None
	}
	if m.Description == None {
		longest := 0
		for c := range fields {
			if c == m.Date || c == m.Time || c == m.Amount || c == m.Debit || c == m.Credit || c == m.Counterparty || c == m.Currency {
				continue
			}
			n := 0
			for _, v := range sample(c) {
				if _, ok := ParseAmount(v, false); !ok {
					n += len(v)
				}
			}
			if n > longest {
				m.Description, longest = c, n
			}
		}
	}

	if m.Date != None {
		if f := GuessDateFormat(sample(m.Date)); f != "" {
			m.DateFormat = f
		}
	}
	var amounts []string
	for _, c := range []int{m.Amount, m.Debit, m.Credit} {
		if c != None {
			amounts = append(amounts, sample(c)...)
		}
	}
	m.DecimalComma = guessDecimalComma(amounts)
	return m
}

func allAmounts(vals []string) bool {
	for _, v := range vals {
		if _, ok := ParseAmount(v, false); !ok {
			return false
		}
	}
	return true
}

var (
	decimalCommaRe = regexp.MustCompile(`,\d{1,2}\)?\s*(?:-|[A-Za-z]{2})?$`)
	decimalDotRe   = regexp.MustCompile(`\.\d{1,2}\)?\s*(?:-|[A-Za-z]{2})?$`)
)

// guessDecimalComma tells whether amounts like 12,50 (and none like 12.50)
// are there.
func guessDecimalComma(vals []string) bool {
	comma := false
	for _, v := range vals {
		if decimalDotRe.MatchString(v) {
			return false
		}
		if decimalCommaRe.MatchString(v) {
			comma = true
		}
	}
	return comma
}

// GuessDateFormat is the first of DateFormats reading every value, ""
// when none does.
func GuessDateFormat(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	for _, f := range DateFormats {
		layout, err := smsparser.DateLayout(f)
		if err != nil {
			continue
		}
		ok := true
		for _, v := range vals {
			if _, _, good := ParseDate(v, layout); !good {
				ok = false
				break
			}
		}
		if ok {
			return f
		}
	}
	return ""
}

// ParseDate reads a date with the layout, maybe followed by its time of
// day (a space or a T between them). The date is midnight UTC, the time of
// day returned apart.
func ParseDate(s, layout string) (time.Time, time.Duration, bool) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return time.Time{}, 0, false
	}
	if t, err := time.Parse(layout, s); err == nil {
		return t, 0, true
	}
	for i := len(s) - 1; i > 0; i-- {
		if s[i] != ' ' && s[i] != 'T' {
			continue
		}
		t, err := time.Parse(layout, strings.TrimSpace(s[:i]))
		if err != nil {
			continue
		}
		clock := strings.TrimSuffix(strings.TrimSpace(s[i+1:]), "Z")
		if j := strings.IndexByte(clock, '.'); j > 0 {
			clock = clock[:j]
		}
		if d, ok := smsparser.ParseClock(clock); ok {
			return t, d, true
		}
	}
	return time.Time{}, 0, false
}

// ParseAmount reads an amount as banks write it: thousands separators,
// currency symbols or codes, negative with a minus (either end), in
// parentheses or with a DR suffix. ok is false when there's no number.
func ParseAmount(s string, decimalComma bool) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	neg := false
	upper := strings.ToUpper(s)
	switch {
	case strings.HasSuffix(upper, "DR"):
		neg, s = true, s[:len(s)-2]
	case strings.HasSuffix(upper, "CR"):
		s = s[:len(s)-2]
	}
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg, s = true, s[1:len(s)-1]
	}
	var b strings.Builder
	digits := 0
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			digits++
		case r == '-' || r == '−':
			neg = true
		case r == '.' || r == ',':
			b.WriteRune(r)
		case unicode.IsSpace(r), r == '\'', r == '+', unicode.IsLetter(r), unicode.IsSymbol(r):
			// separators, currency codes and symbols
		default:
			return 0, false
		}
	}
	if digits == 0 {
		return 0, false
	}
	num := b.String()
	if decimalComma {
		num = strings.ReplaceAll(num, ".", "")
		num = strings.ReplaceAll(num, ",", ".")
	} else {
		num = strings.ReplaceAll(num, ",", "")
	}
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, false
	}
	if neg {
		v = -v
	}
	return v, true
}

// Row is a transaction read off a CSV file.
type Row struct {
	// Line is the row's number among the file's (non-empty) rows, from 1.
	Line int
	// Date is the day, midnight UTC; Clock its time of day when HasTime.
	Date    time.Time
	Clock   time.Duration
	HasTime bool
	// Amount is signed: negative left the account.
	Amount       float64
	Description  string
	Counterparty string
	// Currency is the amount's, upper case, "" for the account's.
	Currency string
	// Hash tells the row imported before (see rowHashes).
	Hash string
	// Err is why it can't be imported, "" when it can.
	Err string
}

// IsOut tells whether the row's money left the account.
func (r *Row) IsOut() bool { return r.Amount < 0 }

// ParseRows reads the transactions of a file into accountID: every data
// row, the ones that can't be imported with their Err. Days after today (the
// user's, midnight UTC) can't be.
func ParseRows(recs [][]string, m *Mapping, accountID string, today time.Time) []Row {
	layout, _ := smsparser.DateLayout(m.DateFormat)
	cell := func(r []string, c int) string {
		if c == None || c >= len(r) {
			return ""
		}
		return strings.Join(strings.Fields(r[c]), " ")
	}

	first := m.SkipRows + 1
	if m.HasHeader {
		first++
	}
	data := m.DataRows(recs)
	rows := make([]Row, 0, len(data))
	for i, r := range data {
		row := Row{
			Line:         first + i,
			Description:  cell(r, m.Description),
			Counterparty: cell(r, m.Counterparty),
			Currency:     strings.ToUpper(cell(r, m.Currency)),
		}

		date, clock, ok := ParseDate(cell(r, m.Date), layout)
		switch {
		case cell(r, m.Date) == "":
			row.Err = "No date"
		case !ok:
			row.Err = "Date not in the format " + m.DateFormat
		case date.After(today):
			row.Err = "Dated ahead"
		}
		row.Date = date
		if t := cell(r, m.Time); t != "" {
			if d, ok := smsparser.ParseClock(t); ok {
				clock = d
			}
		}
		row.Clock, row.HasTime = clock, clock != 0

		amount, ok := m.rowAmount(r, cell)
		row.Amount = amount
		if row.Err == "" {
			switch {
			case !ok:
				row.Err = "No amount"
			case amount == 0:
				row.Err = "Zero amount"
			}
		}
		if row.Currency != "" && (len(row.Currency) != 3 || !isLetters(row.Currency)) {
			row.Currency = ""
		}
		rows = append(rows, row)
	}
	rowHashes(rows, accountID)
	return rows
}

func (m *Mapping) rowAmount(r []string, cell func([]string, int) string) (float64, bool) {
	if !m.Split {
		v, ok := ParseAmount(cell(r, m.Amount), m.DecimalComma)
		if m.Negative == NegativeIn {
			v = -v
		}
		return v, ok
	}
	debit, okD := ParseAmount(cell(r, m.Debit), m.DecimalComma)
	credit, okC := ParseAmount(cell(r, m.Credit), m.DecimalComma)
	return math.Abs(credit) - math.Abs(debit), okD || okC
}

func isLetters(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// rowHashes sets each readable row's hash: its account, day and time,
// amount and description, plus how many same rows came before it in the file
// (two same coffees a day are two transactions), so importing the file (or
// an export overlapping it) again finds them.
func rowHashes(rows []Row, accountID string) {
	seen := map[string]int{}
	for i := range rows {
		r := &rows[i]
		if r.Err != "" {
			continue
		}
		key := fmt.Sprintf("%s|%s|%d|%.6f|%s|%s", accountID, r.Date.Format(time.DateOnly), int64(r.Clock/time.Second),
			r.Amount, r.Currency, strings.ToLower(r.Description))
		n := seen[key]
		seen[key]++
		h := sha256.Sum256(fmt.Appendf(nil, "%s|%d", key, n))
		r.Hash = hex.EncodeToString(h[:])
	}
}

// RowStatus is what a row of a CSV import is to the account.
type RowStatus string

const (
	RowNew RowStatus = "new"
	// RowImported: the same row was imported before.
	RowImported RowStatus = "imported"
	// RowDuplicate: a transaction (e.g. from an SMS) likely is the row.
	RowDuplicate RowStatus = "duplicate"
	RowError     RowStatus = "error"
)

// Existing is a fulfilled transaction moving the account, as a row may
// already be it.
type Existing struct {
	ID string
	// Day is its day as written (time of day dropped).
	Day   time.Time
	Out   bool
	Moved float64
	Hash  string
}

// Match is a row's status and, for a duplicate or an imported one, the
// transaction it is.
type Match struct {
	Status RowStatus
	TxID   string
}

// DuplicateDays is how far apart a row and a transaction it may be can be.
const DuplicateDays = 1

// MatchDuplicates tells each row's status: imported when a transaction
// has its hash, else a duplicate of a transaction moving the account the
// same way by the same amount within DuplicateDays (the closest day), each
// transaction taken by one row at most.
func MatchDuplicates(rows []Row, existing []Existing, cur *models.Currency) []Match {
	res := make([]Match, len(rows))
	used := make([]bool, len(existing))
	byHash := map[string]int{}
	for i, e := range existing {
		if e.Hash != "" {
			byHash[e.Hash] = i
		}
	}

	for i := range rows {
		r := &rows[i]
		if r.Err != "" {
			res[i].Status = RowError
			continue
		}
		if j, ok := byHash[r.Hash]; ok && !used[j] {
			used[j] = true
			res[i] = Match{Status: RowImported, TxID: existing[j].ID}
		}
	}

	round := func(v float64) float64 { return models.RoundCents(v) }
	if cur != nil {
		round = cur.Round
	}
	for i := range rows {
		r := &rows[i]
		if res[i].Status != "" {
			continue
		}
		res[i].Status = RowNew
		if r.Currency != "" && cur != nil && !strings.EqualFold(r.Currency, cur.String) {
			continue
		}
		best, bestDays := -1, DuplicateDays+1
		for j, e := range existing {
			if used[j] || e.Out != r.IsOut() || round(e.Moved) != round(math.Abs(r.Amount)) {
				continue
			}
			days := int(math.Abs(math.Round(e.Day.Sub(r.Date).Hours() / 24)))
			if days < bestDays {
				best, bestDays = j, days
			}
		}
		if best >= 0 {
			used[best] = true
			res[i] = Match{Status: RowDuplicate, TxID: existing[best].ID}
		}
	}
	return res
}
