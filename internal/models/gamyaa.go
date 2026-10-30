package models

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/formatters"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

// Gamyaa is a rotating savings group (جمعية): every month each member pays
// an installment and one of them receives the pot. It's held as an account
// whose installments are transfers into it and payouts transfers out of it.
type Gamyaa struct {
	// Installment is a full share's monthly installment.
	Installment float64
	// Members is also its number of months.
	Members int
	// Shares are the held shares, e.g. 0.5 (split with someone) or 2.
	Shares float64
	// Turns are the 1-based months the pot is received in.
	Turns         []int
	StartsOn      time.Time
	PayFrom       *Account
	RecurrentTxID string

	// From its transactions.
	PaidCount     int
	PaidAmount    float64
	NextPaymentOn time.Time
	ReceivedCount int
	NextPayoutOn  time.Time
}

// MonthlyPayment is what's paid every month for the held shares.
func (g *Gamyaa) MonthlyPayment() float64 {
	return roundCents(g.Installment * g.Shares)
}

// Pot is what's paid in over all the months, and received back in total.
func (g *Gamyaa) Pot() float64 {
	return roundCents(g.MonthlyPayment() * float64(g.Members))
}

// PayoutAmount is what's received on each turn.
func (g *Gamyaa) PayoutAmount() float64 {
	if len(g.Turns) == 0 {
		return 0
	}
	return roundCents(g.Pot() / float64(len(g.Turns)))
}

// MonthDate is the date of its 1-based month n, kept within that month when
// it starts on a day the month doesn't have (e.g. the 31st).
func (g *Gamyaa) MonthDate(n int) time.Time {
	return AddMonthsClamped(g.StartsOn, n-1)
}

// EndsOn is its last installment's date.
func (g *Gamyaa) EndsOn() time.Time {
	return g.MonthDate(g.Members)
}

// IsTurn tells if the pot is received in the 1-based month n.
func (g *Gamyaa) IsTurn(n int) bool {
	return slices.Contains(g.Turns, n)
}

func (g *Gamyaa) IsDone() bool {
	return g.Members > 0 && g.PaidCount >= g.Members && g.ReceivedCount >= len(g.Turns)
}

// Installments are its scheduled transfers from PayFrom into acc.
func (g *Gamyaa) Installments(acc *Account, rt *RecurrentTx, now time.Time) []Transaction {
	txs := make([]Transaction, g.Members)
	for i := range txs {
		txs[i] = Transaction{
			Account:     *g.PayFrom,
			Amount:      g.MonthlyPayment(),
			Kind:        TransferTxKind,
			ToAccount:   acc,
			ToAmount:    g.MonthlyPayment(),
			Description: fmt.Sprintf("%s installment %d/%d", acc.Name, i+1, g.Members),
			RecurrentTx: rt,
			ScheduledTo: g.MonthDate(i + 1),
			CreatedAt:   now,
		}
	}
	return txs
}

// Payouts are its scheduled transfers from acc to PayFrom, one per turn.
func (g *Gamyaa) Payouts(acc *Account, now time.Time) []Transaction {
	turns := slices.Sorted(slices.Values(g.Turns))
	txs := make([]Transaction, len(turns))
	for i, turn := range turns {
		txs[i] = Transaction{
			Account:     *acc,
			Amount:      g.PayoutAmount(),
			Kind:        TransferTxKind,
			ToAccount:   g.PayFrom,
			ToAmount:    g.PayoutAmount(),
			Description: fmt.Sprintf("%s payout (month %d)", acc.Name, turn),
			ScheduledTo: g.MonthDate(turn),
			CreatedAt:   now,
		}
	}
	return txs
}

func (g *Gamyaa) ViewShares() string {
	switch g.Shares {
	case 0.5:
		return "½ share"
	case 1:
		return "1 share"
	}
	return fmt.Sprintf("%s shares", formatters.FormatNum(g.Shares))
}

type createGamyaa struct {
	Installment float64   `validate:"required,gt=0"`
	Members     int       `validate:"required,gte=2,lte=120"`
	Shares      float64   `validate:"required,gt=0"`
	Turns       []int     `validate:"required,min=1,unique,dive,gte=1"`
	StartsOn    time.Time `validate:"required"`
	PayFromID   string    `validate:"required,ulid"`
}

func (g *Gamyaa) SanitizeAndValidate(_ *sanitizer.Sanitizer, valid *valid.Validator) error {
	payFromID := ""
	if g.PayFrom != nil {
		payFromID = g.PayFrom.ID
	}
	cg := &createGamyaa{
		Installment: g.Installment,
		Members:     g.Members,
		Shares:      g.Shares,
		Turns:       g.Turns,
		StartsOn:    g.StartsOn,
		PayFromID:   payFromID,
	}
	// Not sanitized: it has no strings, and conform panics on Turns' []int.
	if valerr := valid.Validate(cg); valerr != nil {
		return *valerr
	}
	// Turns past its months, or more turns than months (shares can't exceed
	// members, every member paying at least one share's installment).
	for _, t := range g.Turns {
		if t > g.Members {
			return errs.ValidationError{Errors: errs.ValidationErrors{
				"Turns": {Tag: "lte", Param: fmt.Sprint(g.Members)},
			}}
		}
	}
	if g.Shares > float64(g.Members) {
		return errs.ValidationError{Errors: errs.ValidationErrors{
			"Shares": {Tag: "lte", Param: fmt.Sprint(g.Members)},
		}}
	}
	return nil
}

// AddMonthsClamped adds n months to t, landing on the target month's last
// day when it doesn't have t's day.
func AddMonthsClamped(t time.Time, n int) time.Time {
	y, m, d := t.Date()
	first := time.Date(y, m+time.Month(n), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	last := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(d, last)-1)
}

func roundCents(f float64) float64 {
	return math.Round(f*100) / 100
}
