package models

import (
	"cmp"
	"math"
	"slices"
	"time"
)

// SubscriptionLookback is how far back DetectSubscriptions is given charges:
// two years, so a yearly one shows twice.
const SubscriptionLookback = 25 // months

// SubscriptionAmountTolerance is how far a charge's amount may be from the
// others' of the same subscription, and how much an existing recurrent
// transaction's or a dismissal's amount may differ from a candidate's for it
// to be the same one.
const SubscriptionAmountTolerance = 0.15

// SubscriptionPriceChange is the change of the last charge against the ones
// before it flagged as a price change.
const SubscriptionPriceChange = 0.05

// SubscriptionTx is a fulfilled expense with a counterparty not made by a
// recurrent transaction, what DetectSubscriptions looks at.
type SubscriptionTx struct {
	ID             string
	AccountID      string
	CurrencyID     string
	CounterpartyID string
	CategoryID     string
	Day            time.Time
	Amount         float64
}

// SubscriptionCandidate is a charge that looks recurring: the same
// counterparty, at a regular interval, for about the same amount, with no
// recurrent transaction making it.
type SubscriptionCandidate struct {
	CounterpartyID string
	CurrencyID     string
	// AccountID and CategoryID are the last charge's (the category the last
	// one with one had).
	AccountID  string
	CategoryID string
	Every      uint
	Unit       RecurrentUnit
	// Amount is the typical charge: the median of the last three.
	Amount float64
	Count  int
	First  time.Time
	Last   time.Time
	// Next is when the next charge is expected: an interval after the last.
	Next time.Time
	// LastAmount is the last charge's, PrevAmount the median of the ones
	// before it; Change their relative difference when the price changed by
	// SubscriptionPriceChange or more, else 0.
	LastAmount float64
	PrevAmount float64
	Change     float64
}

// Key identifies it among the candidates: its counterparty and currency.
func (c *SubscriptionCandidate) Key() string {
	return c.CounterpartyID + "." + c.CurrencyID
}

// Occurrence is the day of the charge k intervals after the last.
func (c *SubscriptionCandidate) Occurrence(k int) time.Time {
	rt := RecurrentTx{Anchor: c.Last, Every: c.Every, Unit: c.Unit}
	return rt.OccurrenceOn(k)
}

// NextFrom is the first expected charge on or after day, what a recurrent
// transaction made of it starts on (the ones before are either paid already
// or were skipped).
func (c *SubscriptionCandidate) NextFrom(day time.Time) time.Time {
	day = dateOnly(day)
	for k := 1; ; k++ {
		if on := c.Occurrence(k); !on.Before(day) {
			return on
		}
	}
}

// PeriodDays is its interval's length in days, on average.
func (c *SubscriptionCandidate) PeriodDays() float64 {
	switch c.Unit {
	case RecurrentUnitDay:
		return float64(c.Every)
	case RecurrentUnitMonth:
		return float64(c.Every) * daysPerMonth
	case RecurrentUnitYear:
		return float64(c.Every) * daysPerYear
	}
	return 0
}

// Monthly is what it comes to in an average month.
func (c *SubscriptionCandidate) Monthly() float64 {
	if d := c.PeriodDays(); d > 0 {
		return c.Amount * daysPerMonth / d
	}
	return 0
}

// SameAs tells whether an amount (an existing recurrent transaction's, a
// dismissal's) is about its typical one: within SubscriptionAmountTolerance.
func (c *SubscriptionCandidate) SameAs(amount float64) bool {
	return withinShare(amount, c.Amount, SubscriptionAmountTolerance)
}

const (
	daysPerYear  = 365.25
	daysPerMonth = daysPerYear / 12
)

// subscriptionPeriod is an interval DetectSubscriptions looks for, with how
// far a gap may be from it (months vary in length, charges slip to the next
// working day).
type subscriptionPeriod struct {
	every uint
	unit  RecurrentUnit
	days  float64
	tol   float64
	// minCharges is how many charges it takes to tell (a yearly one is
	// rarely seen thrice).
	minCharges int
}

var subscriptionPeriods = []subscriptionPeriod{
	{7, RecurrentUnitDay, 7, 1, 3},
	{14, RecurrentUnitDay, 14, 2, 3},
	{1, RecurrentUnitMonth, daysPerMonth, 5, 3},
	{3, RecurrentUnitMonth, 3 * daysPerMonth, 10, 3},
	{6, RecurrentUnitMonth, 6 * daysPerMonth, 15, 3},
	{1, RecurrentUnitYear, daysPerYear, 20, 2},
}

// DetectSubscriptions finds the charges that look recurring among txs (any
// order), grouped by counterparty and currency: a group whose gaps match an
// interval (subscriptionPeriods: two thirds of them at least, none much
// shorter) is a candidate, whatever its amounts (prices go up, bills vary).
// A group that doesn't is split by amount (two plans at one merchant, a one
// off purchase among the charges) and each part tried alike. A candidate
// whose next charge is overdue by more than half an interval as of today
// stopped and is left out. Soonest next charge first.
func DetectSubscriptions(txs []SubscriptionTx, today time.Time) []SubscriptionCandidate {
	today = dateOnly(today)
	groups := map[string][]SubscriptionTx{}
	var keys []string
	for _, tx := range txs {
		if tx.Amount <= 0 || tx.CounterpartyID == "" {
			continue
		}
		k := tx.CounterpartyID + "." + tx.CurrencyID
		if _, ok := groups[k]; !ok {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], tx)
	}

	var out []SubscriptionCandidate
	for _, k := range keys {
		g := byDay(groups[k])
		if c, ok := detectSubscription(g, today); ok {
			out = append(out, c)
			continue
		}
		for _, cluster := range clusterByAmount(g) {
			if c, ok := detectSubscription(cluster, today); ok {
				out = append(out, c)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b SubscriptionCandidate) int {
		if c := a.Next.Compare(b.Next); c != 0 {
			return c
		}
		return cmp.Compare(b.Amount, a.Amount)
	})
	return out
}

// clusterByAmount splits a counterparty's charges into groups of about the
// same amount (each within SubscriptionAmountTolerance of the next smaller
// one), each by day.
func clusterByAmount(txs []SubscriptionTx) [][]SubscriptionTx {
	byAmount := slices.Clone(txs)
	slices.SortStableFunc(byAmount, func(a, b SubscriptionTx) int { return cmp.Compare(a.Amount, b.Amount) })

	var clusters [][]SubscriptionTx
	for _, tx := range byAmount {
		if n := len(clusters); n > 0 {
			if prev := clusters[n-1][len(clusters[n-1])-1]; withinShare(tx.Amount, prev.Amount, SubscriptionAmountTolerance) {
				clusters[n-1] = append(clusters[n-1], tx)
				continue
			}
		}
		clusters = append(clusters, []SubscriptionTx{tx})
	}
	for i, c := range clusters {
		clusters[i] = byDay(c)
	}
	return clusters
}

// byDay is txs sorted by day (then id).
func byDay(txs []SubscriptionTx) []SubscriptionTx {
	s := slices.Clone(txs)
	slices.SortStableFunc(s, func(a, b SubscriptionTx) int {
		if d := a.Day.Compare(b.Day); d != 0 {
			return d
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return s
}

func detectSubscription(txs []SubscriptionTx, today time.Time) (SubscriptionCandidate, bool) {
	if len(txs) < 2 {
		return SubscriptionCandidate{}, false
	}
	gaps := make([]float64, len(txs)-1)
	for i := 1; i < len(txs); i++ {
		gaps[i-1] = dateOnly(txs[i].Day).Sub(dateOnly(txs[i-1].Day)).Hours() / 24
	}
	m := median(gaps)

	var p *subscriptionPeriod
	for i := range subscriptionPeriods {
		if sp := &subscriptionPeriods[i]; math.Abs(m-sp.days) <= sp.tol {
			p = sp
			break
		}
	}
	if p == nil || len(txs) < p.minCharges {
		return SubscriptionCandidate{}, false
	}
	hits := 0
	for _, g := range gaps {
		if g < p.days-p.tol {
			// Charged more often than the interval: not this subscription.
			return SubscriptionCandidate{}, false
		}
		if math.Abs(g-p.days) <= p.tol {
			hits++
		}
	}
	if float64(hits) < float64(len(gaps))*2/3 {
		return SubscriptionCandidate{}, false
	}

	last := txs[len(txs)-1]
	amounts := amountsOf(txs)
	c := SubscriptionCandidate{
		CounterpartyID: last.CounterpartyID,
		CurrencyID:     last.CurrencyID,
		AccountID:      last.AccountID,
		Every:          p.every,
		Unit:           p.unit,
		Amount:         median(amounts[max(0, len(amounts)-3):]),
		Count:          len(txs),
		First:          dateOnly(txs[0].Day),
		Last:           dateOnly(last.Day),
		LastAmount:     last.Amount,
		PrevAmount:     median(amounts[max(0, len(amounts)-4) : len(amounts)-1]),
	}
	for i := len(txs) - 1; i >= 0; i-- {
		if txs[i].CategoryID != "" {
			c.CategoryID = txs[i].CategoryID
			break
		}
	}
	c.Next = c.Occurrence(1)
	// Overdue by more than half an interval: it stopped.
	if today.Sub(c.Next).Hours()/24 > p.days/2+p.tol {
		return SubscriptionCandidate{}, false
	}
	if c.PrevAmount > 0 {
		if ch := (c.LastAmount - c.PrevAmount) / c.PrevAmount; math.Abs(ch) >= SubscriptionPriceChange {
			c.Change = ch
		}
	}
	return c, true
}

func amountsOf(txs []SubscriptionTx) []float64 {
	out := make([]float64, len(txs))
	for i, tx := range txs {
		out[i] = tx.Amount
	}
	return out
}

// median of xs (0 when empty), xs left as is.
func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	if n := len(s); n%2 == 1 {
		return s[n/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// withinShare tells whether a is within share of b (relative to b).
func withinShare(a, b, share float64) bool {
	if b == 0 {
		return a == 0
	}
	return math.Abs(a-b) <= math.Abs(b)*share
}
