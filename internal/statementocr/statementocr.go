// Package statementocr reads banks' PDF statements with Gemini into rows,
// written as a CSV the CSV import (internal/csvimport) maps, previews and
// imports like a bank's export.
package statementocr

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/omareloui/tyche/internal/csvimport"
	"github.com/omareloui/tyche/internal/gemini"
)

const (
	// MaxBytes is the largest PDF read.
	MaxBytes = 10 << 20
	// MaxPages is the most pages a statement may have (when they can be
	// counted).
	MaxPages = 30
	// BalanceTolerance is how far the rows may be off the closing balance.
	BalanceTolerance = 0.01
)

var (
	// ErrNotPDF is a file that isn't a PDF.
	ErrNotPDF = errors.New("the file isn't a PDF")
	// ErrPasswordProtected is an encrypted PDF (no password is asked).
	ErrPasswordProtected = errors.New("the PDF is password protected")
	// ErrTooManyPages is a PDF over MaxPages.
	ErrTooManyPages = errors.New("the PDF has too many pages")
	// ErrUnreadable is a PDF that isn't a statement or couldn't be read.
	ErrUnreadable = errors.New("couldn't read a statement in the PDF")
	// ErrTooLong is a statement whose rows didn't fit one answer.
	ErrTooLong = errors.New("the statement is too long to read at once")
)

// Usage is what a call used, as billed.
type Usage = gemini.Usage

// Reader reads a PDF statement (an interface so tests fake it).
type Reader interface {
	Read(ctx context.Context, pdf []byte) (*Result, Usage, error)
	Model() string
}

// Kind is what a statement is of.
type Kind string

const (
	KindAccount Kind = "account"
	// KindCard: a credit card's, its balances what was owed.
	KindCard Kind = "card"
)

// Row is a transaction of the statement, amounts positive as printed.
type Row struct {
	// Date is YYYY-MM-DD, Time HH:MM or empty.
	Date        string   `json:"date"`
	Time        string   `json:"time"`
	Description string   `json:"description"`
	Debit       float64  `json:"debit"`
	Credit      float64  `json:"credit"`
	Balance     *float64 `json:"balance"`
	// OriginalAmount in OriginalCurrency is what it was paid in when the
	// statement prints it (a purchase abroad), 0 and "" when not.
	OriginalAmount   float64 `json:"original_amount"`
	OriginalCurrency string  `json:"original_currency"`
	// IsFxFee tells a foreign exchange fee line (or its reversal), merged
	// into its purchase by MergeFxFees.
	IsFxFee bool `json:"is_fx_fee"`
	// FxFee is the fee merged into Debit (MergeFxFees), not read.
	FxFee float64 `json:"-"`
}

// Result is what was read off a statement.
type Result struct {
	IsStatement bool   `json:"is_statement"`
	Kind        Kind   `json:"kind"`
	Currency    string `json:"currency"`
	// AccountHint is the account / card number or IBAN as printed (masked
	// digits kept), empty when none.
	AccountHint    string   `json:"account_hint"`
	OpeningBalance *float64 `json:"opening_balance"`
	ClosingBalance *float64 `json:"closing_balance"`
	Rows           []Row    `json:"rows"`
}

// Check tells whether the rows add up from the opening balance to the
// closing one (a card's balances being owed: debits add to them), and by
// how much they're off. ok when either balance is missing.
func (r *Result) Check() (ok bool, diff float64) {
	if r.OpeningBalance == nil || r.ClosingBalance == nil {
		return true, 0
	}
	sum := *r.OpeningBalance
	for _, row := range r.Rows {
		if r.Kind == KindCard {
			sum += row.Debit - row.Credit
		} else {
			sum += row.Credit - row.Debit
		}
	}
	diff = math.Round((*r.ClosingBalance-sum)*100) / 100
	return math.Abs(diff) <= BalanceTolerance, diff
}

// Header is the CSV's header, PresetMapping's columns.
var Header = []string{"date", "time", "description", "debit", "credit", "balance", "original_amount", "original_currency", "fx_fee"}

// FormatVersion is bumped when what Read and ToCSV give changes (columns,
// the prompt): rows read before aren't reused (the cache's key).
const FormatVersion = 2

// CacheKey is the key reading pdf is kept under: its sha256 (hex) and
// FormatVersion.
func CacheKey(pdf []byte) string {
	sum := sha256.Sum256(pdf)
	return fmt.Sprintf("%s:v%d", hex.EncodeToString(sum[:]), FormatVersion)
}

// PresetMappingFor is PresetMapping without the columns a file of columns
// columns lacks (one written by an older ToCSV).
func PresetMappingFor(columns int) csvimport.Mapping {
	m := PresetMapping()
	for _, c := range []*int{&m.Date, &m.Time, &m.Description, &m.Debit, &m.Credit, &m.OriginalAmount, &m.OriginalCurrency, &m.FxFee} {
		if *c >= columns {
			*c = csvimport.None
		}
	}
	return m
}

// PresetMapping reads ToCSV's file.
func PresetMapping() csvimport.Mapping {
	m := csvimport.NewMapping()
	m.Delimiter = ","
	m.HasHeader = true
	m.Date, m.Time, m.Description = 0, 1, 2
	m.Split = true
	m.Debit, m.Credit = 3, 4
	m.DateFormat = "YYYY-MM-DD"
	m.OriginalAmount, m.OriginalCurrency, m.FxFee = 6, 7, 8
	return m
}

// ToCSV writes the rows as a CSV with Header.
func (r *Result) ToCSV() ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(Header); err != nil {
		return nil, err
	}
	num := func(v float64) string {
		if v == 0 {
			return ""
		}
		return strconv.FormatFloat(math.Abs(v), 'f', -1, 64)
	}
	for _, row := range r.Rows {
		bal := ""
		if row.Balance != nil {
			bal = strconv.FormatFloat(*row.Balance, 'f', -1, 64)
		}
		desc := strings.Join(strings.Fields(row.Description), " ")
		origCur := strings.ToUpper(strings.TrimSpace(row.OriginalCurrency))
		origAmt := ""
		if origCur != "" {
			origAmt = num(row.OriginalAmount)
		}
		if err := w.Write([]string{strings.TrimSpace(row.Date), strings.TrimSpace(row.Time), desc, num(row.Debit), num(row.Credit), bal, origAmt, origCur, num(row.FxFee)}); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

var (
	encryptRe = regexp.MustCompile(`/Encrypt\s*\d+\s+\d+\s+R|/Encrypt\s*<<`)
	pageRe    = regexp.MustCompile(`/Type\s*/Page[^s]`)
)

// Inspect checks pdf is a PDF that can be read: not encrypted, at most
// MaxPages pages (when its page objects can be counted: ones packed in
// object streams can't, then it's let through).
func Inspect(pdf []byte) error {
	if !bytes.HasPrefix(bytes.TrimLeft(pdf[:min(len(pdf), 1024)], "\x00\t\r\n "), []byte("%PDF-")) {
		return ErrNotPDF
	}
	if encryptRe.Match(pdf) {
		return ErrPasswordProtected
	}
	if n := len(pageRe.FindAllIndex(pdf, MaxPages+1)); n > MaxPages {
		return ErrTooManyPages
	}
	return nil
}

// IsPDF tells whether data starts like a PDF.
func IsPDF(data []byte) bool {
	return Inspect(data) != ErrNotPDF
}

// Client reads statements with Gemini's generateContent.
type Client struct {
	*gemini.Client
}

// New is a client of the model, nil without an API key.
func New(apiKey, model, thinkingLevel string) *Client {
	c := gemini.New(apiKey, model, thinkingLevel)
	if c == nil {
		return nil
	}
	return &Client{Client: c}
}

const prompt = `You read bank statements (PDF; Arabic, English or both) for a personal finance app: a bank account's or a credit card's.
Extract:
- is_statement: false when the document isn't a bank or card statement listing transactions
- kind: "card" for a credit card statement (its balance is what's owed), else "account"
- currency: the statement's ISO 4217 code (EGP for "Egyptian Pound", "ج.م" or "LE")
- account_hint: the account number, card number or IBAN as printed (keep masked digits as *), empty when none
- opening_balance and closing_balance: as printed (a card's: what was owed), null when not printed; positive unless printed negative
- rows: every transaction in the order printed, on every page:
  - date as YYYY-MM-DD with Western digits: the transaction date, not the posting / value date when both are printed. Dates without a year (e.g. "08-09" or "13 SEP") take the year from the statement's date or period: rows of a month after the statement's month are of the year before (a January statement's December rows). "04OCT26" is 2026-10-04.
  - time as HH:MM (24h) when printed, else empty
  - description: the whole description, its lines joined with spaces (keep names, references and Arabic as printed)
  - debit: the money out of the account (a card's purchases, fees, interest), credit: the money in (a card's payments and refunds, e.g. amounts marked "CR"); positive numbers, 0 for the other one
  - balance: the running balance printed on the row, null when the row has none
  - original_amount and original_currency (ISO 4217): what the row was paid in when the statement prints a foreign amount for it, else 0 and "". A line under a row showing only a foreign amount (e.g. "00-00 00-00 USD6,36 .00": USD 6.36, the comma being the decimal separator there) is that row's original amount, not a row of its own.
  - is_fx_fee: true for a foreign exchange / cross-border fee line charged on a purchase (e.g. "FOREIGN EXCHANGE FEES", "رسوم تحويل عملة") and its reversal on a refund (e.g. "FOREIGN EXCHANGE FEES REV", a credit), even when they're grouped near the end of the statement; keep their own date, which is their purchase's. false for every other row.
Leave out: the opening / closing balance rows, totals, page headers and footers repeated on each page, notes.
Don't invent rows or amounts you can't read.`

var nullableNumber = map[string]any{"type": []string{"number", "null"}}

var responseSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"is_statement":    map[string]any{"type": "boolean"},
		"kind":            map[string]any{"type": "string", "enum": []string{string(KindAccount), string(KindCard)}},
		"currency":        map[string]any{"type": "string"},
		"account_hint":    map[string]any{"type": "string"},
		"opening_balance": nullableNumber,
		"closing_balance": nullableNumber,
		"rows": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"date":              map[string]any{"type": "string"},
					"time":              map[string]any{"type": "string"},
					"description":       map[string]any{"type": "string"},
					"debit":             map[string]any{"type": "number"},
					"credit":            map[string]any{"type": "number"},
					"balance":           nullableNumber,
					"original_amount":   map[string]any{"type": "number"},
					"original_currency": map[string]any{"type": "string"},
					"is_fx_fee":         map[string]any{"type": "boolean"},
				},
				"required": []string{"date", "time", "description", "debit", "credit", "balance", "original_amount", "original_currency", "is_fx_fee"},
			},
		},
	},
	"required": []string{"is_statement", "kind", "currency", "account_hint", "opening_balance", "closing_balance", "rows"},
}

// Read reads the statement in pdf. The usage is returned even on errors
// when the call was billed.
func (c *Client) Read(ctx context.Context, pdf []byte) (*Result, Usage, error) {
	ans, usage, err := c.Generate(ctx, []gemini.Part{gemini.Inline("application/pdf", pdf), {Text: prompt}}, responseSchema)
	if errors.Is(err, gemini.ErrNoAnswer) {
		return nil, usage, fmt.Errorf("%w (%v)", ErrUnreadable, err)
	}
	if err != nil {
		return nil, usage, err
	}
	if ans.FinishReason == "MAX_TOKENS" {
		return nil, usage, ErrTooLong
	}
	var result Result
	if err := json.Unmarshal([]byte(ans.Text), &result); err != nil {
		return nil, usage, fmt.Errorf("%w (finish reason %s)", ErrUnreadable, ans.FinishReason)
	}
	if !result.IsStatement || len(result.Rows) == 0 {
		return &result, usage, ErrUnreadable
	}
	return &result, usage, nil
}
