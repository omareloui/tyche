package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

type RecurrentUnit uint8

const (
	RecurrentUnitNil RecurrentUnit = iota
	RecurrentUnitDay
	RecurrentUnitMonth
	RecurrentUnitYear
)

func (fb RecurrentUnit) String() string {
	return [...]string{"???", "Day", "Month", "Year"}[fb]
}

func RecurrentUnits() []RecurrentUnit {
	return []RecurrentUnit{RecurrentUnitDay, RecurrentUnitMonth, RecurrentUnitYear}
}

type RecurrentTx struct {
	ID    string
	Unit  RecurrentUnit
	Every uint

	Account      Account
	Amount       float64
	Kind         TxKind
	Description  string
	ToAmount     float64
	ToAccount    *Account
	Category     *Category
	Counterparty *Counterparty

	StartsAt                   time.Time
	StopsAt                    time.Time
	CreatedAt                  time.Time
	LastTransactionScheduledTo time.Time

	// Upcoming are its scheduled, not yet fulfilled, transactions' dates,
	// soonest first.
	Upcoming []time.Time
	// FulfilledCount and LastFulfilledOn are about its fulfilled ones.
	FulfilledCount  int
	LastFulfilledOn time.Time
}

// AddOccurrence records one of its generated transactions, passed in
// scheduled date order.
func (rt *RecurrentTx) AddOccurrence(scheduledTo time.Time, fulfilled bool) {
	if fulfilled {
		rt.FulfilledCount++
		rt.LastFulfilledOn = scheduledTo
		return
	}
	rt.Upcoming = append(rt.Upcoming, scheduledTo)
}

// HasEnded tells if it stopped and has nothing scheduled left.
func (rt *RecurrentTx) HasEnded(now time.Time) bool {
	return !rt.StopsAt.IsZero() && rt.StopsAt.Before(now) && len(rt.Upcoming) == 0
}

// NextOn is the date of the next transaction it generates, after the last
// scheduled one.
func (rt *RecurrentTx) NextOn() time.Time {
	last := rt.LastTransactionScheduledTo
	var next time.Time
	switch rt.Unit {
	case RecurrentUnitDay:
		next = last.AddDate(0, 0, int(rt.Every))
	case RecurrentUnitMonth:
		next = last.AddDate(0, int(rt.Every), 0)
	case RecurrentUnitYear:
		next = last.AddDate(int(rt.Every), 0, 0)
	}
	// A day the month lacks (e.g. the 31st) falls on its last day.
	if rt.Unit != RecurrentUnitDay && next.Day() != last.Day() {
		next = next.AddDate(0, 0, -next.Day())
	}
	return next.Truncate(24 * time.Hour)
}

// maxOccurrences caps OccurrencesBetween, e.g. a daily one last scheduled
// long ago.
const maxOccurrences = 5000

// OccurrencesBetween are the dates of the transactions it would generate
// next (after its last scheduled one), after after and before until, until
// it stops: the ones it hasn't scheduled yet.
func (rt *RecurrentTx) OccurrencesBetween(after, until time.Time) []time.Time {
	if rt.Every == 0 || rt.LastTransactionScheduledTo.IsZero() {
		return nil
	}
	c := *rt
	var res []time.Time
	for range maxOccurrences {
		next := c.NextOn()
		if !next.Before(until) || (!rt.StopsAt.IsZero() && next.After(rt.StopsAt)) {
			break
		}
		if next.After(after) {
			res = append(res, next)
		}
		c.LastTransactionScheduledTo = next
	}
	return res
}

// FullyScheduled tells if every transaction until it stops is scheduled, so
// there's no more to schedule.
func (rt *RecurrentTx) FullyScheduled() bool {
	return !rt.StopsAt.IsZero() && rt.NextOn().After(rt.StopsAt)
}

// PerMonth is how many times it happens in an average month.
func (rt *RecurrentTx) PerMonth() float64 {
	if rt.Every == 0 {
		return 0
	}
	every := float64(rt.Every)
	switch rt.Unit {
	case RecurrentUnitDay:
		return 365.25 / 12 / every
	case RecurrentUnitMonth:
		return 1 / every
	case RecurrentUnitYear:
		return 1 / (12 * every)
	}
	return 0
}

// AsTransaction is a transaction as it generates them, to show it like one.
func (rt *RecurrentTx) AsTransaction() *Transaction {
	return &Transaction{
		ID:           rt.ID,
		Account:      rt.Account,
		Amount:       rt.Amount,
		Kind:         rt.Kind,
		Category:     rt.Category,
		Counterparty: rt.Counterparty,
		ToAccount:    rt.ToAccount,
		ToAmount:     rt.ToAmount,
		Description:  rt.Description,
		RecurrentTx:  rt,
	}
}

type createRecurrentTx struct {
	commonFieldsForTxAndRtx
	Unit  RecurrentUnit `validate:"required"`
	Every uint          `validate:"required"`

	StartsAt                   time.Time `validate:"required"`
	StopsAt                    time.Time `validate:"omitempty,gtfield=StartsAt"`
	LastTransactionScheduledTo time.Time `validate:"required"`
}

func (rt *RecurrentTx) ViewRecurrence() string {
	if rt.Every == 1 {
		switch rt.Unit {
		case RecurrentUnitDay:
			return "Daily"
		case RecurrentUnitMonth:
			return "Monthly"
		case RecurrentUnitYear:
			return "Yearly"
		}
	}
	return fmt.Sprintf("Every %d %ss", rt.Every, strings.ToLower(rt.Unit.String()))
}

func (rt *RecurrentTx) ViewAmount() string {
	return rt.Account.Currency.Format(rt.Amount)
}

func (rt *RecurrentTx) ViewToAmount() string {
	if rt.ToAccount == nil {
		return ""
	}
	return rt.ToAccount.Currency.Format(rt.ToAmount)
}

func (rt *RecurrentTx) toValidate() *createRecurrentTx {
	var catId, toAccId, cpId string

	if cat := rt.Category; cat != nil {
		catId = cat.ID
	}
	if cp := rt.Counterparty; cp != nil {
		cpId = cp.ID
	}
	if toAcc := rt.ToAccount; toAcc != nil {
		toAccId = toAcc.ID
	}
	return &createRecurrentTx{
		commonFieldsForTxAndRtx: commonFieldsForTxAndRtx{
			ID:             rt.ID,
			AccountID:      rt.Account.ID,
			Amount:         rt.Amount,
			Kind:           uint8(rt.Kind),
			CategoryID:     catId,
			CounterpartyID: cpId,
			ToAccountID:    toAccId,
			ToAmount:       rt.ToAmount,
			Description:    rt.Description,
		},
		Unit:                       rt.Unit,
		Every:                      rt.Every,
		StartsAt:                   rt.StartsAt,
		StopsAt:                    rt.StopsAt,
		LastTransactionScheduledTo: rt.LastTransactionScheduledTo,
	}
}

func (rt *RecurrentTx) SanitizeAndValidate(sani *sanitizer.Sanitizer, valid *valid.Validator) error {
	cRt := rt.toValidate()
	if err := sani.SanitizeStruct(cRt); err != nil {
		return errs.ErrSanitizer
	}
	if valerr := valid.Validate(cRt); valerr != nil {
		return *valerr
	}
	return nil
}
