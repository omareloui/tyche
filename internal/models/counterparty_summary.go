package models

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

// DayMerchantTotal is a merchant's spending of a day in a currency.
type DayMerchantTotal struct {
	Day          time.Time
	Counterparty Counterparty
	Currency     Currency
	Amount       float64
}

// LabelSpend is what was spent at the merchants with a label; Label is empty
// for the ones without any.
type LabelSpend struct {
	Label     string
	Amount    float64
	Merchants int
}

// CounterpartySummary is a month's spending per merchant (most first) and
// per label, in Currency. Hidden merchants are left out of every total.
type CounterpartySummary struct {
	MerchantSpending
	// Hidden are the hidden merchants spent at in the month, most first.
	Hidden  []MerchantSpend
	Labels  []LabelSpend
	Partial bool
}

// NewCounterpartySummary is p's spending per merchant from totals, in def.
func NewCounterpartySummary(p Period, def Currency, totals []DayMerchantTotal, hidden []string) *CounterpartySummary {
	cs := &CounterpartySummary{MerchantSpending: MerchantSpending{Month: p.Month, Period: p, Currency: def}}

	var items []MerchantSpend
	idx := map[string]int{}
	for _, t := range totals {
		if !p.Contains(t.Day) {
			continue
		}
		amount, ok := Convert(t.Amount, &t.Currency, def.String)
		if !ok {
			cs.Partial = true
			continue
		}
		if i, ok := idx[t.Counterparty.ID]; ok {
			items[i].Amount += amount
			continue
		}
		idx[t.Counterparty.ID] = len(items)
		items = append(items, MerchantSpend{Counterparty: t.Counterparty, Amount: amount})
	}

	// Merchants whose refunds cover their expenses are dropped.
	items = slices.DeleteFunc(items, func(it MerchantSpend) bool { return it.Amount <= 0 })
	slices.SortStableFunc(items, func(a, b MerchantSpend) int { return cmp.Compare(b.Amount, a.Amount) })
	for _, it := range items {
		if slices.Contains(hidden, it.Counterparty.ID) {
			cs.Hidden = append(cs.Hidden, it)
		} else {
			cs.Items = append(cs.Items, it)
		}
	}

	cs.Labels = labelSpending(cs.Items)
	return cs
}

// labelSpending sums items per label (a merchant counting in each of its
// labels), written as first seen, most first, the unlabeled ones last.
func labelSpending(items []MerchantSpend) []LabelSpend {
	var res []LabelSpend
	idx := map[string]int{}
	var unlabeled LabelSpend
	for _, it := range items {
		seen := map[string]bool{}
		for _, l := range it.Counterparty.Labels {
			key := strings.ToLower(strings.TrimSpace(l))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			i, ok := idx[key]
			if !ok {
				i = len(res)
				idx[key] = i
				res = append(res, LabelSpend{Label: strings.TrimSpace(l)})
			}
			res[i].Amount += it.Amount
			res[i].Merchants++
		}
		if len(seen) == 0 {
			unlabeled.Amount += it.Amount
			unlabeled.Merchants++
		}
	}
	slices.SortStableFunc(res, func(a, b LabelSpend) int { return cmp.Compare(b.Amount, a.Amount) })
	if unlabeled.Merchants > 0 {
		res = append(res, unlabeled)
	}
	return res
}

// HasLabels tells whether any merchant of the month has a label.
func (cs *CounterpartySummary) HasLabels() bool {
	return len(cs.Labels) > 0 && cs.Labels[0].Label != ""
}

// LabelBarPct is the label's bar width, as a percentage of the top label's.
func (cs *CounterpartySummary) LabelBarPct(amount float64) float64 {
	var top float64
	for _, l := range cs.Labels {
		top = max(top, l.Amount)
	}
	if top <= 0 {
		return 0
	}
	return max(amount, 0) / top * 100
}
