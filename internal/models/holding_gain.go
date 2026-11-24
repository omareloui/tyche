package models

import (
	"math"
	"strings"
	"time"
)

// HoldingAccount is an account in another currency than the default one
// (dollars, gold, a goal's holding) whose cost is walked: its current
// balance, in its own currency (troy ounces for metals), and when it was
// opened.
type HoldingAccount struct {
	ID      string
	Code    string
	Balance float64
	Opened  time.Time
}

// HoldingMove is a fulfilled transaction moving a holding account, with
// both sides' currency codes. PairAmount / PairCode are a transfer pair's
// other leg (two SMS of a transfer between own accounts), zero otherwise.
type HoldingMove struct {
	Day         time.Time
	Kind        TxKind
	AccountID   string
	ToAccountID string
	Amount      float64
	// ToAmount is what reached the to account, 0 = Amount.
	ToAmount   float64
	FromCode   string
	ToCode     string
	PairAmount float64
	PairCode   string
}

func (m *HoldingMove) received() float64 {
	if m.ToAmount != 0 {
		return m.ToAmount
	}
	return m.Amount
}

func (m *HoldingMove) moves() bool {
	return m.ToAccountID != "" && (m.Kind == TransferTxKind || m.Kind == DebtTxKind)
}

// HoldingGain is what a holding cost in the default currency (average
// cost: every inflow at what was paid for it, outflows taking their share
// of it), what it's worth at today's rate and the gain.
type HoldingGain struct {
	Code string
	// Units is what it holds, in its own currency.
	Units float64
	// Basis is what the units held cost, in the default currency.
	Basis float64
	// Rate is today's rate to the default currency, 0 when there's none
	// (Value and the gains then unknown).
	Rate       float64
	Value      float64
	Unrealized float64
	// Realized is the gain (or loss) of what went out: what it fetched less
	// what it cost.
	Realized float64
	// Estimated tells some cost is a guess: an opening balance without
	// history, or a day's rate that couldn't be had (today's used).
	Estimated bool
}

// AvgCost is what a unit cost on average, in the default currency.
func (g *HoldingGain) AvgCost() float64 {
	if g.Units <= 0 {
		return 0
	}
	return g.Basis / g.Units
}

// UnrealizedPct is the unrealized gain as a percentage of the cost.
func (g *HoldingGain) UnrealizedPct() float64 {
	if g.Basis <= 0 {
		return 0
	}
	return g.Unrealized / g.Basis * 100
}

// Shows tells there's a gain to show: units held at a known rate and cost.
func (g *HoldingGain) Shows() bool {
	return g != nil && g.Units > holdingEps && g.Rate > 0 && g.Basis > 0
}

// holdingEps is how close to 0 units count as none (a gram's hundredths
// are ~3e-4 troy ounces).
const holdingEps = 1e-7

// HoldingRateFunc is a code's rate to the default currency on a day; ok is
// false when it isn't that day's (today's used instead).
type HoldingRateFunc func(day time.Time, code string) (rate float64, ok bool)

type holdingState struct {
	units, basis, realized float64
	estimated              bool
}

// out takes amount units out, returning the basis they carry; a part of it
// beyond the units held carries none.
func (s *holdingState) out(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	var removed float64
	if s.units > holdingEps {
		removed = s.basis * min(amount, s.units) / s.units
	}
	s.units -= amount
	s.basis -= removed
	if s.units <= holdingEps {
		if s.units > -holdingEps {
			s.units = 0
		}
		s.basis = 0
	}
	return removed
}

// covered is the share of amount the units held cover.
func (s *holdingState) covered(amount float64) float64 {
	if amount <= 0 || s.units <= holdingEps {
		return 0
	}
	return min(amount, s.units) / amount
}

// in adds amount units costing cost; the part filling units owed (below
// 0) carries no cost.
func (s *holdingState) in(amount, cost float64) {
	if amount <= 0 {
		return
	}
	if s.units < 0 {
		cost *= max(0, s.units+amount) / amount
	}
	s.units += amount
	s.basis += cost
}

// sell takes amount units out for proceeds (in the default currency),
// booking the gain of the part held.
func (s *holdingState) sell(amount, proceeds float64) {
	c := s.covered(amount)
	removed := s.out(amount)
	s.realized += proceeds*c - removed
}

// ComputeHoldingGains walks the moves (in order) of the holding accounts
// (every account in another currency than def, by code, so money moved
// between two of the same currency keeps its cost) at average cost:
//   - what comes in costs what left the other side when it's in def,
//     else its amount at the day's rate (fees on transfers part of it);
//   - a transfer between two holdings of the same currency moves its share
//     of the cost, no gain;
//   - what goes out takes its share of the cost and books what it fetched
//     (what reached a def account, else the day's rate) less it.
//
// An opening balance the moves don't explain (an account created with one)
// costs its amount at the rate of its first day, Estimated. today is a
// code's rate today. By account id.
func ComputeHoldingGains(def string, accs []HoldingAccount, moves []HoldingMove, rateOn HoldingRateFunc, today func(code string) float64) map[string]*HoldingGain {
	def = strings.ToUpper(def)
	codes := make(map[string]string, len(accs))
	for _, a := range accs {
		codes[a.ID] = strings.ToUpper(a.Code)
	}

	// The opening balances: the balance less what the moves did.
	effect := make(map[string]float64, len(accs))
	first := make(map[string]time.Time, len(accs))
	seen := func(id string, day time.Time) {
		if f, ok := first[id]; !ok || day.Before(f) {
			first[id] = day
		}
	}
	for i := range moves {
		m := &moves[i]
		if _, ok := codes[m.AccountID]; ok {
			seen(m.AccountID, m.Day)
			switch m.Kind {
			case IncomeTxKind, RefundTxKind, AdjustmentTxKind:
				effect[m.AccountID] += m.Amount
			default:
				effect[m.AccountID] -= m.Amount
			}
		}
		if _, ok := codes[m.ToAccountID]; ok && m.moves() {
			seen(m.ToAccountID, m.Day)
			effect[m.ToAccountID] += m.received()
		}
	}

	state := make(map[string]*holdingState, len(accs))
	rate := func(st *holdingState, day time.Time, code string) float64 {
		r, ok := rateOn(day, code)
		if !ok {
			st.estimated = true
		}
		return r
	}
	for _, a := range accs {
		st := &holdingState{}
		state[a.ID] = st
		opening := a.Balance - effect[a.ID]
		if math.Abs(opening) <= holdingEps {
			continue
		}
		st.estimated = true
		day := a.Opened
		if f, ok := first[a.ID]; ok && (day.IsZero() || f.Before(day)) {
			day = f
		}
		if opening > 0 {
			st.in(opening, opening*rate(st, day, codes[a.ID]))
		} else {
			st.units = opening
		}
	}

	for i := range moves {
		m := &moves[i]
		from, fromHeld := state[m.AccountID]
		to, toHeld := state[m.ToAccountID]
		if !m.moves() {
			if !fromHeld {
				continue
			}
			code := codes[m.AccountID]
			inflow := m.Kind == IncomeTxKind || m.Kind == RefundTxKind || (m.Kind == AdjustmentTxKind && m.Amount > 0)
			amount := m.Amount
			if m.Kind == AdjustmentTxKind {
				amount = math.Abs(amount)
			}
			paired := m.PairAmount > 0 && strings.EqualFold(m.PairCode, def)
			switch {
			case inflow && paired:
				from.in(amount, m.PairAmount)
			case inflow:
				from.in(amount, amount*rate(from, m.Day, code))
			case paired:
				from.sell(amount, m.PairAmount)
			default:
				from.sell(amount, amount*rate(from, m.Day, code))
			}
			continue
		}

		received := m.received()
		switch {
		case fromHeld && toHeld && codes[m.AccountID] == codes[m.ToAccountID]:
			// Between two holdings of the same currency: the cost moves.
			c := from.covered(m.Amount)
			basis := from.out(m.Amount)
			// What reached it beyond the cost carried (units owed) costs
			// the day's rate.
			if c < 1 {
				basis += received * (1 - c) * rate(to, m.Day, codes[m.ToAccountID])
			}
			to.in(received, basis)
			to.estimated = to.estimated || from.estimated
		case fromHeld:
			var proceeds float64
			if strings.EqualFold(m.ToCode, def) {
				proceeds = received
			} else {
				proceeds = m.Amount * rate(from, m.Day, codes[m.AccountID])
			}
			from.sell(m.Amount, proceeds)
			if toHeld {
				to.in(received, proceeds)
				to.estimated = to.estimated || from.estimated
			}
		case toHeld:
			var cost float64
			if strings.EqualFold(m.FromCode, def) {
				cost = m.Amount
			} else {
				cost = m.Amount * rate(to, m.Day, m.FromCode)
			}
			to.in(received, cost)
		}
	}

	gains := make(map[string]*HoldingGain, len(accs))
	for _, a := range accs {
		st := state[a.ID]
		g := &HoldingGain{Code: codes[a.ID], Units: st.units, Basis: st.basis, Realized: st.realized, Estimated: st.estimated}
		g.finish(today(g.Code))
		gains[a.ID] = g
	}
	return gains
}

func (g *HoldingGain) finish(rate float64) {
	g.Rate = rate
	if rate <= 0 {
		g.Value, g.Unrealized = 0, 0
		return
	}
	g.Value = g.Units * rate
	g.Unrealized = g.Value - g.Basis
}

// HoldingRateNeeds are the days and codes ComputeHoldingGains asks rates
// of, to fetch them at once.
func HoldingRateNeeds(def string, accs []HoldingAccount, moves []HoldingMove) (days []time.Time, codes []string) {
	seenDay := map[time.Time]bool{}
	seenCode := map[string]bool{}
	ComputeHoldingGains(def, accs, moves, func(day time.Time, code string) (float64, bool) {
		day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
		if !seenDay[day] {
			seenDay[day] = true
			days = append(days, day)
		}
		if code = strings.ToUpper(code); !seenCode[code] {
			seenCode[code] = true
			codes = append(codes, code)
		}
		return 1, true
	}, func(string) float64 { return 1 })
	return days, codes
}

// SumHoldingGains sums gains of one currency (the accounts the summary
// counts in it).
func SumHoldingGains(gains ...*HoldingGain) *HoldingGain {
	s := &HoldingGain{}
	for _, g := range gains {
		if g == nil {
			continue
		}
		s.Code = g.Code
		s.Units += g.Units
		s.Basis += g.Basis
		s.Realized += g.Realized
		s.Estimated = s.Estimated || g.Estimated
		s.Rate = g.Rate
	}
	s.finish(s.Rate)
	return s
}
