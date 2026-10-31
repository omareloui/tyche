// Package smsingest turns parsed bank SMS into transactions.
package smsingest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/omareloui/tyche/internal/formatters"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

var ErrUnmatchedAccount = errors.New("couldn't match the sms to an account")

// cashRef is what parsers set as the account id for cash withdrawals/deposits.
const cashRef = "Cash"

// minRefDigits is the least amount of trailing digits needed to match an
// account by its card number or iban.
const minRefDigits = 3

// FxRateFunc returns the rate to convert from a currency to another.
type FxRateFunc func(from, to string) (float64, error)

// Hash identifies an SMS to avoid ingesting it twice.
func Hash(sender, body string, receivedAt time.Time) string {
	h := sha256.Sum256(fmt.Appendf(nil, "%s|%s|%d", strings.TrimSpace(sender), strings.TrimSpace(body), receivedAt.Unix()))
	return hex.EncodeToString(h[:])
}

// BuildTransaction creates a fulfilled transaction flagged for review from a
// parsed SMS, at the sms date (and time) in loc when it has one.
func BuildTransaction(
	sms *smsparser.SMS,
	accounts []models.Account,
	counterparties []models.Counterparty,
	receivedAt time.Time,
	loc *time.Location,
	fxRate FxRateFunc,
) (*models.Transaction, error) {
	from := matchAccount(sms.AccountID, sms.Currency, accounts)
	to := matchAccount(sms.ToAccountID, toCurrency(sms), accounts)

	kind := sms.TxKind
	var acc *models.Account

	switch kind {
	case models.TransferTxKind:
		switch {
		case from != nil && to != nil:
			acc = from
		case from != nil:
			kind, acc = models.ExpenseTxKind, from
		case to != nil:
			kind, acc = models.IncomeTxKind, to
		}
	case models.IncomeTxKind:
		// income parsers set the receiving account as AccountID
		acc = from
	default:
		acc = from
	}

	if acc == nil {
		return nil, ErrUnmatchedAccount
	}

	at := TxTime(sms, receivedAt, loc)
	tx := &models.Transaction{
		ID:          idgen.New(),
		Account:     *acc,
		Kind:        kind,
		ScheduledTo: at,
		FulfilledAt: at,
		NeedsReview: true,
	}

	var notes []string
	if sms.Note != "" {
		notes = append(notes, sms.Note)
	}

	amount, note, err := convert(sms.Amount, sms.Currency, acc, fxRate)
	if err != nil {
		return nil, err
	}
	tx.Amount = amount
	if note != "" {
		notes = append(notes, note)
	}

	if kind == models.TransferTxKind {
		toAmount := sms.ToAmount
		if toAmount == 0 {
			toAmount = sms.Amount
		}
		toAmount, note, err := convert(toAmount, toCurrency(sms), to, fxRate)
		if err != nil {
			return nil, err
		}
		tx.ToAccount = to
		tx.ToAmount = toAmount
		if note != "" {
			notes = append(notes, note)
		}
	} else if cp := matchCounterparty(sms.Counterparty, counterparties); cp != nil {
		tx.Counterparty = cp
		tx.Category = cp.DefaultCategoryFor(kind)
	} else if name := strings.TrimSpace(sms.Counterparty); name != "" && sms.AutoCreateCounterparty {
		// no ID, the caller creates it
		tx.Counterparty = &models.Counterparty{Name: name, IsMerchant: sms.CounterpartyIsMerchant}
	} else if sms.Counterparty != "" {
		notes = append([]string{sms.Counterparty}, notes...)
	}

	if kind != sms.TxKind {
		if ref := otherRef(sms, kind); ref != "" {
			notes = append(notes, fmt.Sprintf("%s %s", sms.TxKind, ref))
		}
	}

	tx.Description = strings.Join(notes, " · ")

	return tx, nil
}

// maxClockSkew is how far after receiving it an sms date can be, more is
// taken as a misread date.
const maxClockSkew = time.Hour

// TxTime is when the sms says the transaction happened, in loc: its date at
// its time, or at the time it was received when it has no time. Dates without
// a year are in the last 12 months. Falls back to receivedAt when the sms has
// no date or it's in the future.
func TxTime(sms *smsparser.SMS, receivedAt time.Time, loc *time.Location) time.Time {
	d := sms.Timestamp
	if d.IsZero() {
		return receivedAt
	}

	local := receivedAt.In(loc)
	year := d.Year()
	if year == 0 {
		year = local.Year()
	}

	h, m, s := local.Clock()
	if sms.HasTime {
		h, m, s = d.Clock()
	}
	t := time.Date(year, d.Month(), d.Day(), h, m, s, 0, loc)

	limit := receivedAt.Add(maxClockSkew)
	if d.Year() == 0 && t.After(limit) {
		t = t.AddDate(-1, 0, 0)
	}
	if t.After(limit) {
		return receivedAt
	}
	return t
}

// otherRef is the side of the transfer that didn't match an account.
func otherRef(sms *smsparser.SMS, kind models.TxKind) string {
	if kind == models.ExpenseTxKind {
		return "to " + sms.ToAccountID
	}
	return "from " + sms.AccountID
}

func toCurrency(sms *smsparser.SMS) string {
	if sms.ToCurrency != "" {
		return sms.ToCurrency
	}
	return sms.Currency
}

func convert(amount float64, cur string, acc *models.Account, fxRate FxRateFunc) (float64, string, error) {
	if cur == "" || strings.EqualFold(cur, acc.Currency.String) {
		return amount, "", nil
	}
	rate, err := fxRate(cur, acc.Currency.String)
	if err != nil {
		return 0, "", fmt.Errorf("getting fx rate from %s to %s: %w", cur, acc.Currency.String, err)
	}
	if rate == 0 {
		return 0, "", fmt.Errorf("got zero fx rate from %s to %s", cur, acc.Currency.String)
	}
	return amount * rate, "originally " + formatters.FormatAmount(amount, cur), nil
}

func matchAccount(ref, cur string, accounts []models.Account) *models.Account {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}

	if ref == cashRef {
		return pick(accounts, cur, func(a *models.Account) bool {
			return a.Kind.Enum == models.AccKindCash
		})
	}

	nref := normalize(ref)
	if acc := pick(accounts, cur, func(a *models.Account) bool {
		return a.SmsRef != "" && normalize(a.SmsRef) == nref
	}); acc != nil {
		return acc
	}

	tail := trailingDigits(ref)
	if len(tail) < minRefDigits {
		return nil
	}

	return pick(accounts, cur, func(a *models.Account) bool {
		for _, s := range []string{a.SmsRef, a.CardNumber, a.Iban} {
			if s != "" && strings.HasSuffix(digits(s), tail) {
				return true
			}
		}
		return false
	})
}

// pick returns the only active account matching, preferring the ones in the
// sms currency when many match. Returns nil when it's ambiguous.
func pick(accounts []models.Account, cur string, match func(*models.Account) bool) *models.Account {
	var matched []*models.Account
	for i := range accounts {
		if !accounts[i].IsArchive && match(&accounts[i]) {
			matched = append(matched, &accounts[i])
		}
	}

	if len(matched) > 1 && cur != "" {
		var sameCur []*models.Account
		for _, a := range matched {
			if strings.EqualFold(a.Currency.String, cur) {
				sameCur = append(sameCur, a)
			}
		}
		if len(sameCur) > 0 {
			matched = sameCur
		}
	}

	if len(matched) != 1 {
		return nil
	}
	acc := *matched[0]
	return &acc
}

func matchCounterparty(name string, counterparties []models.Counterparty) *models.Counterparty {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	for i := range counterparties {
		if strings.EqualFold(strings.TrimSpace(counterparties[i].Name), name) {
			cp := counterparties[i]
			return &cp
		}
	}
	for i := range counterparties {
		for _, alias := range counterparties[i].Aliases {
			if strings.EqualFold(strings.TrimSpace(alias), name) {
				cp := counterparties[i]
				return &cp
			}
		}
	}
	return nil
}

func normalize(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '-' {
			return -1
		}
		return unicode.ToUpper(r)
	}, s)
}

func digits(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}

// trailingDigits returns the digits after the masked part, e.g. "789" for
// "123-456***-789" and "1234" for "**1234".
func trailingDigits(s string) string {
	if i := strings.LastIndex(s, "*"); i >= 0 {
		s = s[i+1:]
	}
	return digits(s)
}
