package models

import (
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TxStatus narrows transactions by whether they're done or need review.
type TxStatus string

const (
	TxStatusAny       TxStatus = ""
	TxStatusScheduled TxStatus = "scheduled"
	TxStatusFulfilled TxStatus = "fulfilled"
	TxStatusReview    TxStatus = "review"
)

// TxSource narrows transactions by how they were added.
type TxSource string

const (
	TxSourceAny    TxSource = ""
	TxSourceSMS    TxSource = "sms"
	TxSourceImport TxSource = "import"
	TxSourceManual TxSource = "manual"
)

// TxReceipt narrows transactions by whether they have a receipt's image.
type TxReceipt string

const (
	TxReceiptAny     TxReceipt = ""
	TxReceiptWith    TxReceipt = "with"
	TxReceiptWithout TxReceipt = "without"
)

// Query keys of the transactions filters, also the form field names.
const (
	TxFilterQ            = "q"
	TxFilterKind         = "kind"
	TxFilterAccount      = "account"
	TxFilterCategory     = "category"
	TxFilterCounterparty = "counterparty"
	TxFilterGoal         = "goal"
	TxFilterStatus       = "status"
	TxFilterSource       = "source"
	TxFilterReceipt      = "receipt"
	TxFilterFrom         = "from"
	TxFilterTo           = "to"
	TxFilterMin          = "min"
	TxFilterMax          = "max"
)

// TxFilterNone is the value of the category and counterparty filters
// matching the transactions without one.
const TxFilterNone = UncategorizedKey

const txFilterDate = "2006-01-02"

// TxFilter is what the transactions list is narrowed by, kept in the page's
// query string. The zero value matches everything.
type TxFilter struct {
	Q              string
	Kind           TxKind
	AccountID      string
	CategoryID     string
	CounterpartyID string
	GoalID         string
	Status         TxStatus
	Source         TxSource
	Receipt        TxReceipt
	// From and To are inclusive days, zero for no bound.
	From, To time.Time
	// MinAmount and MaxAmount bound the amount, in the account's currency.
	MinAmount, MaxAmount *float64
}

// ParseTxFilter reads the filter from a query string, ignoring bad values.
func ParseTxFilter(v url.Values) TxFilter {
	f := TxFilter{
		Q:              strings.TrimSpace(v.Get(TxFilterQ)),
		Kind:           GetTxKindFromString(v.Get(TxFilterKind)),
		AccountID:      strings.TrimSpace(v.Get(TxFilterAccount)),
		CategoryID:     strings.TrimSpace(v.Get(TxFilterCategory)),
		CounterpartyID: strings.TrimSpace(v.Get(TxFilterCounterparty)),
		GoalID:         strings.TrimSpace(v.Get(TxFilterGoal)),
	}
	switch s := TxStatus(v.Get(TxFilterStatus)); s {
	case TxStatusScheduled, TxStatusFulfilled, TxStatusReview:
		f.Status = s
	}
	switch s := TxSource(v.Get(TxFilterSource)); s {
	case TxSourceSMS, TxSourceImport, TxSourceManual:
		f.Source = s
	}
	switch s := TxReceipt(v.Get(TxFilterReceipt)); s {
	case TxReceiptWith, TxReceiptWithout:
		f.Receipt = s
	}
	f.From = parseFilterDate(v.Get(TxFilterFrom))
	f.To = parseFilterDate(v.Get(TxFilterTo))
	f.MinAmount = parseFilterAmount(v.Get(TxFilterMin))
	f.MaxAmount = parseFilterAmount(v.Get(TxFilterMax))
	return f
}

func parseFilterDate(s string) time.Time {
	t, err := time.Parse(txFilterDate, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}
	}
	return t
}

// parseFilterAmount reads an amount, maybe typed with a currency symbol and
// thousands separators.
func parseFilterAmount(s string) *float64 {
	s = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r == '.' || r == '-' {
			return r
		}
		return -1
	}, s)
	if s == "" {
		return nil
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &n
}

// Values is the filter as a query string, without the empty ones.
func (f *TxFilter) Values() url.Values {
	v := url.Values{}
	set := func(k, val string) {
		if val != "" {
			v.Set(k, val)
		}
	}
	set(TxFilterQ, f.Q)
	if f.Kind != NilTxKind {
		set(TxFilterKind, f.Kind.String())
	}
	set(TxFilterAccount, f.AccountID)
	set(TxFilterCategory, f.CategoryID)
	set(TxFilterCounterparty, f.CounterpartyID)
	set(TxFilterGoal, f.GoalID)
	set(TxFilterStatus, string(f.Status))
	set(TxFilterSource, string(f.Source))
	set(TxFilterReceipt, string(f.Receipt))
	set(TxFilterFrom, f.FromString())
	set(TxFilterTo, f.ToString())
	set(TxFilterMin, FilterAmountString(f.MinAmount))
	set(TxFilterMax, FilterAmountString(f.MaxAmount))
	return v
}

// Without is the filter's query string without the given keys, e.g. to
// clear one of them.
func (f *TxFilter) Without(keys ...string) string {
	v := f.Values()
	for _, k := range keys {
		v.Del(k)
	}
	return v.Encode()
}

func (f *TxFilter) IsZero() bool {
	return len(f.Values()) == 0
}

// ActiveCount is how many filters besides the search are set, the date range
// and amount range counting once each.
func (f *TxFilter) ActiveCount() int {
	v := f.Values()
	n := 0
	for _, k := range []string{TxFilterKind, TxFilterAccount, TxFilterCategory, TxFilterCounterparty, TxFilterGoal, TxFilterStatus, TxFilterSource, TxFilterReceipt} {
		if v.Has(k) {
			n++
		}
	}
	if v.Has(TxFilterFrom) || v.Has(TxFilterTo) {
		n++
	}
	if v.Has(TxFilterMin) || v.Has(TxFilterMax) {
		n++
	}
	return n
}

// KindValue is the kind as in the query string, "" for any.
func (f *TxFilter) KindValue() string {
	if f.Kind == NilTxKind {
		return ""
	}
	return f.Kind.String()
}

func (f *TxFilter) FromString() string {
	if f.From.IsZero() {
		return ""
	}
	return f.From.Format(txFilterDate)
}

func (f *TxFilter) ToString() string {
	if f.To.IsZero() {
		return ""
	}
	return f.To.Format(txFilterDate)
}

func FilterAmountString(n *float64) string {
	if n == nil {
		return ""
	}
	return strconv.FormatFloat(*n, 'f', -1, 64)
}

// TxPage is a chunk of the transactions list.
type TxPage struct {
	// Upcoming are the unfulfilled transactions, only in the first chunk.
	Upcoming []Transaction
	// History are the fulfilled ones, newest first, whole (user's) months.
	History []Transaction
	// Until is the first day of the oldest month loaded so far, zero when
	// everything is.
	Until time.Time
	// HasMore tells if there are older transactions, from Until back.
	HasMore bool
	// NeedsReview counts all the transactions to review, unfiltered.
	NeedsReview int
	// MonthStart is the day the user's months, the list's sections, start
	// on.
	MonthStart MonthStart
}

// TxSummary is what a filter of the transactions list matches as a whole,
// in the default currency.
type TxSummary struct {
	// Count is how many rows the list shows; Scheduled of them aren't
	// fulfilled yet.
	Count, Scheduled int
	// In and Out are what the fulfilled ones earn and spend, following the
	// analysis rule.
	In, Out float64
	// Partial tells some currencies had no fx rate, left out of In and Out.
	Partial bool
}
