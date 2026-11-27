package models

import (
	"cmp"
	"math"
	"slices"
	"time"
)

// ChargeLine is a charge line (VAT, service, tip, ...) of a fulfilled
// expense, as GetChargeLines reads it: its amounts are in the lines'
// currency, brought to the account's (Currency) with the transaction's own
// rate (ItemsRate).
type ChargeLine struct {
	TxID     string
	Day      time.Time
	Kind     TxItemKind
	Total    float64
	Rate     *float64
	Included bool
	// Subtotal is what its transaction's items add up to, the base of its
	// rate.
	Subtotal float64
	// Foreign tells the lines are in another currency than the account's.
	Foreign bool
	// LinesTotal is what the transaction's lines add up to (TxItemsTotal),
	// Amount its amount.
	LinesTotal, Amount float64
	Currency           Currency
	// Merchant is its transaction's counterparty, nil when none.
	Merchant *Counterparty
}

// ChargeKindStat is what a kind of charge came to, in the default
// currency.
type ChargeKindStat struct {
	Kind  TxItemKind
	Total float64
	Count int
	// Rate is its average percentage of the items' subtotal (weighted by
	// it), 0 when none is known.
	Rate float64
	// rated sums rate × base, base the subtotals of the rated lines.
	rated, base float64
}

// HasRate tells its average rate is known.
func (k *ChargeKindStat) HasRate() bool { return k.Rate > 0 }

// ChargeMerchantStat is what the charges came to at a merchant.
type ChargeMerchantStat struct {
	// Merchant is nil for the transactions without a counterparty.
	Merchant *Counterparty
	Total    float64
	Receipts int
	// Kinds are most first.
	Kinds []ChargeKindStat
}

// ChargeMonth is what the charges came to in a user month.
type ChargeMonth struct {
	Period Period
	Total  float64
	// Kinds are in TxItemChargeKinds' order, only the ones it had.
	Kinds []ChargeKindStat
}

// ChargeStats is what the receipts' charges came to, in the default
// currency at today's rates. Like the items, they're lines, never money
// moved: nothing here counts as spending anywhere else.
type ChargeStats struct {
	Currency Currency
	// Total is the charges on top of the prices, discounts netted.
	Total float64
	// Receipts is how many transactions have charges on top, ReceiptsSpent
	// what they cost.
	Receipts      int
	ReceiptsSpent float64
	// Kinds are the charges on top of the prices, most first.
	Kinds []ChargeKindStat
	// Included are the ones already inside the prices (shown, not summed).
	Included []ChargeKindStat
	// Merchants are most first.
	Merchants []ChargeMerchantStat
	// Months are every user month from the first charge's to the last's,
	// oldest first.
	Months []ChargeMonth
	// Partial tells some lines were left out, their currency having no rate.
	Partial bool
}

// Empty tells there's no charge at all.
func (s *ChargeStats) Empty() bool { return len(s.Kinds) == 0 && len(s.Included) == 0 }

// Share is the charges' percentage of what their receipts cost.
func (s *ChargeStats) Share() float64 {
	if s.ReceiptsSpent <= 0 {
		return 0
	}
	return s.Total / s.ReceiptsSpent * 100
}

// Kind is the stat of a kind on top of the prices, nil when none.
func (s *ChargeStats) Kind(k TxItemKind) *ChargeKindStat {
	for i := range s.Kinds {
		if s.Kinds[i].Kind == k {
			return &s.Kinds[i]
		}
	}
	return nil
}

// chargeRated are the kinds whose rate is worked out of the subtotal when
// the receipt doesn't show it.
func chargeRated(k TxItemKind) bool {
	return k == TxItemVAT || k == TxItemService || k == TxItemTip
}

type chargeKinds map[TxItemKind]*ChargeKindStat

func (m chargeKinds) add(k TxItemKind, total, rate, base float64) {
	st := m[k]
	if st == nil {
		st = &ChargeKindStat{Kind: k}
		m[k] = st
	}
	st.Total += total
	st.Count++
	if rate > 0 && base > 0 {
		st.rated += rate * base
		st.base += base
	}
}

// list are the kinds, rounded in def, most first (by size: a discount
// is as big as it takes off).
func (m chargeKinds) list(def Currency) []ChargeKindStat {
	out := make([]ChargeKindStat, 0, len(m))
	for _, k := range TxItemChargeKinds() {
		if st := m[k]; st != nil {
			st.Total = def.Round(st.Total)
			if st.base > 0 {
				st.Rate = math.Round(st.rated/st.base*10) / 10
			}
			out = append(out, *st)
		}
	}
	slices.SortStableFunc(out, func(a, b ChargeKindStat) int {
		return cmp.Compare(math.Abs(b.Total), math.Abs(a.Total))
	})
	return out
}

// NewChargeStats sums lines (oldest first) in def, by the user's months
// (ms).
func NewChargeStats(lines []ChargeLine, def Currency, ms MonthStart) *ChargeStats {
	s := &ChargeStats{Currency: def}
	kinds, included := chargeKinds{}, chargeKinds{}
	type merchant struct {
		st    *ChargeMerchantStat
		kinds chargeKinds
		txs   map[string]bool
	}
	merchants := map[string]*merchant{}
	var order []string
	months := map[string]chargeKinds{}
	monthTotals := map[string]float64{}
	var first, last time.Time
	txs := map[string]bool{}

	for _, l := range lines {
		ratio := ItemsRate(l.LinesTotal, l.Amount, l.Foreign)
		total, ok := Convert(l.Total*ratio, &l.Currency, def.String)
		if !ok {
			s.Partial = true
			continue
		}
		base, _ := Convert(l.Subtotal*ratio, &l.Currency, def.String)
		rate := 0.0
		switch {
		case l.Rate != nil:
			rate = *l.Rate
		case chargeRated(l.Kind) && !l.Included && l.Subtotal > 0:
			rate = math.Abs(l.Total) / l.Subtotal * 100
		}
		if l.Included {
			included.add(l.Kind, total, rate, base)
			continue
		}
		kinds.add(l.Kind, total, rate, base)
		s.Total += total
		if !txs[l.TxID] {
			txs[l.TxID] = true
			s.Receipts++
			if amt, ok := Convert(math.Abs(l.Amount), &l.Currency, def.String); ok {
				s.ReceiptsSpent += amt
			}
		}

		key := ""
		if l.Merchant != nil {
			key = l.Merchant.ID
		}
		m := merchants[key]
		if m == nil {
			m = &merchant{st: &ChargeMerchantStat{Merchant: l.Merchant}, kinds: chargeKinds{}, txs: map[string]bool{}}
			merchants[key] = m
			order = append(order, key)
		}
		m.st.Total += total
		m.kinds.add(l.Kind, total, rate, base)
		if !m.txs[l.TxID] {
			m.txs[l.TxID] = true
			m.st.Receipts++
		}

		p := ms.Of(l.Day)
		if first.IsZero() || p.Month.Before(first) {
			first = p.Month
		}
		if p.Month.After(last) {
			last = p.Month
		}
		mk := months[p.Key()]
		if mk == nil {
			mk = chargeKinds{}
			months[p.Key()] = mk
		}
		mk.add(l.Kind, total, 0, 0)
		monthTotals[p.Key()] += total
	}

	s.Total = def.Round(s.Total)
	s.ReceiptsSpent = def.Round(s.ReceiptsSpent)
	s.Kinds = kinds.list(def)
	s.Included = included.list(def)
	for _, key := range order {
		m := merchants[key]
		m.st.Total = def.Round(m.st.Total)
		m.st.Kinds = m.kinds.list(def)
		s.Merchants = append(s.Merchants, *m.st)
	}
	slices.SortStableFunc(s.Merchants, func(a, b ChargeMerchantStat) int { return cmp.Compare(b.Total, a.Total) })
	if !first.IsZero() {
		for mo := first; !mo.After(last); mo = mo.AddDate(0, 1, 0) {
			p := ms.For(mo)
			cm := ChargeMonth{Period: p, Total: def.Round(monthTotals[p.Key()])}
			if mk := months[p.Key()]; mk != nil {
				for _, k := range TxItemChargeKinds() {
					if st := mk[k]; st != nil {
						cm.Kinds = append(cm.Kinds, ChargeKindStat{Kind: k, Total: def.Round(st.Total), Count: st.Count})
					}
				}
			}
			s.Months = append(s.Months, cm)
		}
	}
	return s
}
