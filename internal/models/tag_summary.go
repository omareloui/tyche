package models

import (
	"sort"
	"time"
)

// TagTopCategories is how many categories a tag's card shows.
const TagTopCategories = 3

// CurrencyAmount is an amount in a currency.
type CurrencyAmount struct {
	Currency Currency
	Amount   float64
}

// TagSpend is what a tag's fulfilled expenses spent on a day (scheduled_to's,
// YYYY-MM-DD) in a category ("" = uncategorized) and currency, following the
// analysis rule.
type TagSpend struct {
	TagID      string
	Day        string
	CategoryID string
	Currency   Currency
	Amount     float64
}

// TagCategorySpend is what a tag spent in a category (nil = uncategorized),
// in the default currency.
type TagCategorySpend struct {
	Category *Category
	Amount   float64
}

// TagDaySpend is what a tag spent on a day, in the default currency.
type TagDaySpend struct {
	Day    time.Time
	Amount float64
}

// TagSummary is what a tag spent: in the default currency at today's rates,
// per currency as spent, per category and per day.
type TagSummary struct {
	Tag      Tag
	Currency Currency
	Spent    float64
	// Partial tells some currencies had no rate to the default one (left
	// out of Spent).
	Partial    bool
	ByCurrency []CurrencyAmount
	ByCategory []TagCategorySpend
	ByDay      []TagDaySpend
	// Budget is the tag's budget in the default currency, 0 when none (or
	// no rate).
	Budget float64
}

// BudgetUsed is the share of the budget spent (over 1 when over it), 0
// without a budget.
func (s *TagSummary) BudgetUsed() float64 {
	if s.Budget <= 0 {
		return 0
	}
	return s.Spent / s.Budget
}

// OverBudget tells it spent more than its budget.
func (s *TagSummary) OverBudget() bool { return s.Budget > 0 && s.Spent > s.Budget }

// TopCategories are its first n categories by spending.
func (s *TagSummary) TopCategories(n int) []TagCategorySpend {
	if len(s.ByCategory) <= n {
		return s.ByCategory
	}
	return s.ByCategory[:n]
}

// MaxDay is its biggest day's spending, for the chart's scale.
func (s *TagSummary) MaxDay() float64 {
	var m float64
	for _, d := range s.ByDay {
		m = max(m, d.Amount)
	}
	return m
}

// NewTagSummaries sums the spends of each tag (by id), every tag having one
// (empty when it spent nothing). cats are the categories by id.
func NewTagSummaries(tags []Tag, spends []TagSpend, cats map[string]Category, def Currency) map[string]*TagSummary {
	out := make(map[string]*TagSummary, len(tags))
	for _, t := range tags {
		s := &TagSummary{Tag: t, Currency: def}
		if t.Budget != nil && t.BudgetCurrency != nil {
			if b, ok := Convert(*t.Budget, t.BudgetCurrency, def.String); ok {
				s.Budget = def.Round(b)
			}
		}
		out[t.ID] = s
	}
	byCur := map[string]map[string]int{}
	byCat := map[string]map[string]float64{}
	byDay := map[string]map[string]float64{}
	for _, sp := range spends {
		s, ok := out[sp.TagID]
		if !ok {
			continue
		}
		if byCur[sp.TagID] == nil {
			byCur[sp.TagID] = map[string]int{}
			byCat[sp.TagID] = map[string]float64{}
			byDay[sp.TagID] = map[string]float64{}
		}
		i, ok := byCur[sp.TagID][sp.Currency.ID]
		if !ok {
			i = len(s.ByCurrency)
			byCur[sp.TagID][sp.Currency.ID] = i
			s.ByCurrency = append(s.ByCurrency, CurrencyAmount{Currency: sp.Currency})
		}
		s.ByCurrency[i].Amount += sp.Amount
		amount, ok := Convert(sp.Amount, &sp.Currency, def.String)
		if !ok {
			s.Partial = true
			continue
		}
		s.Spent += amount
		byCat[sp.TagID][sp.CategoryID] += amount
		byDay[sp.TagID][sp.Day] += amount
	}
	for id, s := range out {
		s.Spent = def.Round(s.Spent)
		for i := range s.ByCurrency {
			s.ByCurrency[i].Amount = s.ByCurrency[i].Currency.Round(s.ByCurrency[i].Amount)
		}
		sort.SliceStable(s.ByCurrency, func(i, j int) bool { return s.ByCurrency[i].Amount > s.ByCurrency[j].Amount })
		for catID, amount := range byCat[id] {
			cs := TagCategorySpend{Amount: def.Round(amount)}
			if c, ok := cats[catID]; ok {
				cs.Category = &c
			}
			s.ByCategory = append(s.ByCategory, cs)
		}
		sort.Slice(s.ByCategory, func(i, j int) bool {
			if s.ByCategory[i].Amount != s.ByCategory[j].Amount {
				return s.ByCategory[i].Amount > s.ByCategory[j].Amount
			}
			return tagCatName(s.ByCategory[i]) < tagCatName(s.ByCategory[j])
		})
		for day, amount := range byDay[id] {
			d, err := time.Parse(time.DateOnly, day)
			if err != nil {
				continue
			}
			s.ByDay = append(s.ByDay, TagDaySpend{Day: d, Amount: def.Round(amount)})
		}
		sort.Slice(s.ByDay, func(i, j int) bool { return s.ByDay[i].Day.Before(s.ByDay[j].Day) })
	}
	return out
}

func tagCatName(c TagCategorySpend) string {
	if c.Category == nil {
		return ""
	}
	return c.Category.Name
}
