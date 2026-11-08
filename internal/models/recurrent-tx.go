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
	// Items are its lines, copied to every occurrence.
	Items []TxItem

	StartsAt  time.Time
	StopsAt   time.Time
	CreatedAt time.Time

	// Anchor is the day its occurrences are counted from (occurrence 0, see
	// OccurrenceOn), NextIndex the next one to write.
	Anchor    time.Time
	NextIndex int
	// NextUnset tells NextIndex wasn't recomputed since the anchors were
	// migrated from the last scheduled dates: it continues from today when
	// that's past.
	NextUnset bool
	// AutoFulfill fulfills its occurrences on their day by themselves.
	AutoFulfill bool
	PausedAt    time.Time
	// GamyaaAccountID is the gam'yaa whose installments it pays, set on its
	// creation and only changed through its transactions.
	GamyaaAccountID string

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

func (rt *RecurrentTx) IsPaused() bool {
	return !rt.PausedAt.IsZero()
}

// IsGamyaa tells it pays a gam'yaa's installments: locked, as the gam'yaa's
// schedule is only set on its creation.
func (rt *RecurrentTx) IsGamyaa() bool {
	return rt.GamyaaAccountID != ""
}

// OccurrenceOn is the day of its occurrence k: k × every units after its
// anchor, a day the month lacks (e.g. the 31st) falling on its last day, so
// the next months are back on the anchor's day.
func (rt *RecurrentTx) OccurrenceOn(k int) time.Time {
	a := dateOnly(rt.Anchor)
	n := k * int(rt.Every)
	switch rt.Unit {
	case RecurrentUnitDay:
		return a.AddDate(0, 0, n)
	case RecurrentUnitMonth:
		return addMonthsClamped(a, n)
	case RecurrentUnitYear:
		return addMonthsClamped(a, 12*n)
	}
	return a
}

// addMonthsClamped is n months after t, on t's day or the month's last.
func addMonthsClamped(t time.Time, n int) time.Time {
	first := time.Date(t.Year(), t.Month()+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	return first.AddDate(0, 0, min(t.Day(), daysIn(first))-1)
}

// NextOn is the day of the next occurrence it writes.
func (rt *RecurrentTx) NextOn() time.Time {
	return rt.OccurrenceOn(rt.NextIndex)
}

// longestUnitDays is the most days one of its intervals can last, to start
// counting occurrences close to a day.
func (rt *RecurrentTx) longestUnitDays() int {
	switch rt.Unit {
	case RecurrentUnitMonth:
		return 31 * int(rt.Every)
	case RecurrentUnitYear:
		return 366 * int(rt.Every)
	}
	return int(rt.Every)
}

// ResumeIndex is its first occurrence on or after day.
func (rt *RecurrentTx) ResumeIndex(day time.Time) int {
	if rt.Every == 0 {
		return 0
	}
	day = dateOnly(day)
	k := max(0, int(day.Sub(dateOnly(rt.Anchor)).Hours()/24)/rt.longestUnitDays()-1)
	for rt.OccurrenceOn(k).Before(day) {
		k++
	}
	return k
}

// ContinueFrom makes one migrated from its last scheduled date (NextUnset)
// continue from today when that's past: the occurrences missed meanwhile are
// skipped, not written as overdue.
func (rt *RecurrentTx) ContinueFrom(today time.Time) {
	if rt.NextUnset {
		rt.NextIndex = max(rt.NextIndex, rt.ResumeIndex(today))
		rt.NextUnset = false
	}
}

// ScheduleHorizon is how far ahead the occurrences are written: through the
// user's next month.
func ScheduleHorizon(ms MonthStart, today time.Time) time.Time {
	return ms.Of(today).Next().Last()
}

// maxOccurrences caps the occurrences counted at once, e.g. a daily one last
// written long ago.
const maxOccurrences = 5000

// ToSchedule are the indexes of the occurrences to write now, from the next
// one: the ones through horizon, and one after today if none is, until it
// stops. None while paused.
func (rt *RecurrentTx) ToSchedule(today, horizon time.Time) []int {
	if rt.Every == 0 || rt.IsPaused() || rt.Anchor.IsZero() {
		return nil
	}
	today = dateOnly(today)
	var res []int
	for k := rt.NextIndex; k < rt.NextIndex+maxOccurrences; k++ {
		d := rt.OccurrenceOn(k)
		if !rt.StopsAt.IsZero() && d.After(dateOnly(rt.StopsAt)) {
			break
		}
		if d.After(horizon) && k > 0 && rt.OccurrenceOn(k-1).After(today) {
			break
		}
		res = append(res, k)
	}
	return res
}

// OccurrencesBetween are the dates of the transactions it would generate
// next (from its next index), after after and before until, until it stops:
// the ones it hasn't written yet. None while paused.
func (rt *RecurrentTx) OccurrencesBetween(after, until time.Time) []time.Time {
	if rt.Every == 0 || rt.IsPaused() || rt.Anchor.IsZero() {
		return nil
	}
	k := rt.NextIndex
	if rt.NextUnset {
		k = max(k, rt.ResumeIndex(after))
	}
	var res []time.Time
	for end := k + maxOccurrences; k < end; k++ {
		next := rt.OccurrenceOn(k)
		if !next.Before(until) || (!rt.StopsAt.IsZero() && next.After(rt.StopsAt)) {
			break
		}
		if next.After(after) {
			res = append(res, next)
		}
	}
	return res
}

// FullyScheduled tells if every transaction until it stops is scheduled, so
// there's no more to schedule.
func (rt *RecurrentTx) FullyScheduled() bool {
	return !rt.StopsAt.IsZero() && rt.NextOn().After(rt.StopsAt)
}

// Occurrence is its occurrence k as a scheduled transaction.
func (rt *RecurrentTx) Occurrence(id string, k int, now time.Time) Transaction {
	return Transaction{
		ID:           id,
		Account:      rt.Account,
		Amount:       rt.Amount,
		Kind:         rt.Kind,
		Category:     rt.Category,
		Counterparty: rt.Counterparty,
		ToAccount:    rt.ToAccount,
		ToAmount:     rt.ToAmount,
		Description:  rt.Description,
		Items:        CopyTxItems(rt.Items),
		RecurrentTx:  &RecurrentTx{ID: rt.ID},
		ScheduledTo:  rt.OccurrenceOn(k),
		CreatedAt:    now,
	}
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

	StartsAt time.Time `validate:"required"`
	StopsAt  time.Time `validate:"omitempty,gtfield=StartsAt"`
	Anchor   time.Time `validate:"required"`
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
			Items:          rt.Items,
		},
		Unit:     rt.Unit,
		Every:    rt.Every,
		StartsAt: rt.StartsAt,
		StopsAt:  rt.StopsAt,
		Anchor:   rt.Anchor,
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
