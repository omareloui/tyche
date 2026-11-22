package models

import (
	"cmp"
	"slices"
	"time"
)

// ItemLine is an item line of a fulfilled expense, as the items analyses
// read it (GetItemLines): its amounts are in the lines' currency, brought to
// the account's (Currency) with the transaction's own rate (ItemsRate).
type ItemLine struct {
	ItemID   string
	TxID     string
	Day      time.Time
	Quantity float64
	Total    float64
	// Foreign tells the lines are in another currency than the account's.
	Foreign bool
	// LinesTotal is what the transaction's lines add up to (TxItemsTotal),
	// Amount its amount.
	LinesTotal, Amount float64
	Currency           Currency
	// Merchant is its transaction's counterparty, nil when none.
	Merchant *Counterparty
}

// ItemPurchase is one line of an item, in the default currency.
type ItemPurchase struct {
	TxID      string
	Day       time.Time
	Merchant  *Counterparty
	Quantity  float64
	UnitPrice float64
	Total     float64
}

// ItemMerchantPrice is what an item cost at one merchant.
type ItemMerchantPrice struct {
	Merchant *Counterparty
	Times    int
	Qty      float64
	Spent    float64
	// Last, Min and Avg are unit prices (Avg weighted by quantity).
	Last, Min, Avg float64
	LastOn         time.Time
}

// ItemStats is what was bought of an item, in the default currency at
// today's rates. Lines are never money moved: nothing here counts as
// spending anywhere else.
type ItemStats struct {
	Currency Currency
	Times    int
	Qty      float64
	Spent    float64
	// Unit prices: the first and last bought, the lowest, highest and the
	// average (weighted by quantity).
	First, Last, Min, Max, Avg float64
	LastOn                     time.Time
	// Purchases are newest first.
	Purchases []ItemPurchase
	// PerMerchant are most spent first; the ones without a merchant under a
	// nil Merchant.
	PerMerchant []ItemMerchantPrice
	// Partial tells some lines were left out, their currency having no rate.
	Partial bool
}

// ChangePct is how much the unit price moved from the first purchase to
// the last, in percent (0 with fewer than two).
func (s *ItemStats) ChangePct() float64 {
	if s.Times < 2 || s.First == 0 {
		return 0
	}
	return (s.Last - s.First) / s.First * 100
}

// Multiple tells there's more than one purchase to compare.
func (s *ItemStats) Multiple() bool { return s.Times > 1 }

// itemPurchase is the line in def, false when its currency has no rate.
func itemPurchase(l ItemLine, def Currency) (ItemPurchase, bool) {
	total := l.Total * ItemsRate(l.LinesTotal, l.Amount, l.Foreign)
	total, ok := Convert(total, &l.Currency, def.String)
	if !ok {
		return ItemPurchase{}, false
	}
	qty := l.Quantity
	if qty <= 0 {
		qty = 1
	}
	return ItemPurchase{
		TxID: l.TxID, Day: l.Day, Merchant: l.Merchant, Quantity: qty,
		UnitPrice: def.Round(total / qty), Total: def.Round(total),
	}, true
}

// NewItemStats sums lines (oldest first, all of one item) in def.
func NewItemStats(lines []ItemLine, def Currency) *ItemStats {
	s := &ItemStats{Currency: def}
	merchants := map[string]*ItemMerchantPrice{}
	var order []string
	for _, l := range lines {
		p, ok := itemPurchase(l, def)
		if !ok {
			s.Partial = true
			continue
		}
		s.add(p)

		key := ""
		if p.Merchant != nil {
			key = p.Merchant.ID
		}
		m := merchants[key]
		if m == nil {
			m = &ItemMerchantPrice{Merchant: p.Merchant, Min: p.UnitPrice}
			merchants[key] = m
			order = append(order, key)
		}
		m.Times++
		m.Qty += p.Quantity
		m.Spent += p.Total
		m.Last, m.LastOn = p.UnitPrice, p.Day
		m.Min = min(m.Min, p.UnitPrice)
	}
	s.finish()
	for _, key := range order {
		m := merchants[key]
		if m.Qty > 0 {
			m.Avg = def.Round(m.Spent / m.Qty)
		}
		m.Spent = def.Round(m.Spent)
		s.PerMerchant = append(s.PerMerchant, *m)
	}
	slices.SortStableFunc(s.PerMerchant, func(a, b ItemMerchantPrice) int { return cmp.Compare(b.Spent, a.Spent) })
	return s
}

func (s *ItemStats) add(p ItemPurchase) {
	if s.Times == 0 {
		s.First, s.Min, s.Max = p.UnitPrice, p.UnitPrice, p.UnitPrice
	}
	s.Times++
	s.Qty += p.Quantity
	s.Spent += p.Total
	s.Last, s.LastOn = p.UnitPrice, p.Day
	s.Min, s.Max = min(s.Min, p.UnitPrice), max(s.Max, p.UnitPrice)
	s.Purchases = append(s.Purchases, p)
}

func (s *ItemStats) finish() {
	if s.Qty > 0 {
		s.Avg = s.Currency.Round(s.Spent / s.Qty)
	}
	s.Spent = s.Currency.Round(s.Spent)
	slices.Reverse(s.Purchases)
}

// ItemSummary is an item of the items page with its stats.
type ItemSummary struct {
	Item      Item
	Merchants int
	Stats     *ItemStats
}

// NewItemSummaries pairs every item with the stats of its lines (any
// order of items, lines oldest first), keeping the items' order.
func NewItemSummaries(items []ItemSummary, lines []ItemLine, def Currency) []ItemSummary {
	byItem := map[string][]ItemLine{}
	for _, l := range lines {
		byItem[l.ItemID] = append(byItem[l.ItemID], l)
	}
	for i := range items {
		items[i].Stats = NewItemStats(byItem[items[i].Item.ID], def)
		// The list doesn't show them: keep it light.
		items[i].Stats.Purchases = nil
		items[i].Stats.PerMerchant = nil
	}
	return items
}
