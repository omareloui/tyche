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

// TagDaySpend is what a tag spent on a day, in the default currency, per
// category in the summary's ByCategory order.
type TagDaySpend struct {
	Day        time.Time
	Amount     float64
	ByCategory []TagCategorySpend
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
	byDayCat := map[string]map[string]map[string]float64{}
	for _, sp := range spends {
		s, ok := out[sp.TagID]
		if !ok {
			continue
		}
		if byCur[sp.TagID] == nil {
			byCur[sp.TagID] = map[string]int{}
			byCat[sp.TagID] = map[string]float64{}
			byDay[sp.TagID] = map[string]float64{}
			byDayCat[sp.TagID] = map[string]map[string]float64{}
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
		if byDayCat[sp.TagID][sp.Day] == nil {
			byDayCat[sp.TagID][sp.Day] = map[string]float64{}
		}
		byDayCat[sp.TagID][sp.Day][sp.CategoryID] += amount
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
			ds := TagDaySpend{Day: d, Amount: def.Round(amount)}
			for _, c := range s.ByCategory {
				if a := byDayCat[id][day][tagCatID(c)]; a != 0 {
					ds.ByCategory = append(ds.ByCategory, TagCategorySpend{Category: c.Category, Amount: def.Round(a)})
				}
			}
			s.ByDay = append(s.ByDay, ds)
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

func tagCatID(c TagCategorySpend) string {
	if c.Category == nil {
		return ""
	}
	return c.Category.ID
}

// TagMonthOption is a month the tag spent in, for the month picker.
type TagMonthOption struct {
	Period Period
	Amount float64
}

// TagRange is what a tag spent over some days, in the default currency.
type TagRange struct {
	From, To   time.Time // inclusive
	Amount     float64
	ByCategory []TagCategorySpend
}

// Days counts its days.
func (r *TagRange) Days() int { return int(r.To.Sub(r.From).Hours()/24) + 1 }

// TagMonth is a tag's spending over one of the user's months: every day of it
// (the ones without spending too), what it spent in all and per category,
// and the range of days picked in it.
type TagMonth struct {
	Period Period
	Days   []TagDaySpend
	// Max is its biggest day, the chart's scale.
	Max   float64
	Month TagRange
	// Selection is the range picked, nil when none.
	Selection *TagRange
	// Prev and Next are the neighbor months' keys, "" past the first or
	// last month the tag spent in.
	Prev, Next string
	// Months are the months it spent in, newest first.
	Months []TagMonthOption
}

// Shown is the range the breakdown is of: the selection, else the month.
func (m *TagMonth) Shown() *TagRange {
	if m.Selection != nil {
		return m.Selection
	}
	return &m.Month
}

// Selected tells whether a day is in the selection.
func (m *TagMonth) Selected(d time.Time) bool {
	return m.Selection != nil && !d.Before(m.Selection.From) && !d.After(m.Selection.To)
}

// DefaultMonth is the month the details open on: today's when the tag spent
// in it, else the one of its last spending (today's when none).
func (s *TagSummary) DefaultMonth(start MonthStart, today time.Time) Period {
	cur := start.Of(today)
	if len(s.ByDay) == 0 {
		return cur
	}
	for _, d := range s.ByDay {
		if cur.Contains(d.Day) {
			return cur
		}
	}
	return start.Of(s.ByDay[len(s.ByDay)-1].Day)
}

// Month is the tag's spending over p, with the days from..to (inclusive,
// clamped to p, swapped when reversed) picked when both are set.
func (s *TagSummary) Month(p Period, from, to time.Time) *TagMonth {
	m := &TagMonth{Period: p, Month: TagRange{From: p.From, To: p.Last()}}
	byDay := make(map[time.Time]TagDaySpend, len(s.ByDay))
	for _, d := range s.ByDay {
		byDay[d.Day] = d
	}
	if !from.IsZero() && !to.IsZero() {
		from, to = dateOnly(from), dateOnly(to)
		if to.Before(from) {
			from, to = to, from
		}
		if from.Before(p.From) {
			from = p.From
		}
		if to.After(p.Last()) {
			to = p.Last()
		}
		if !to.Before(from) {
			m.Selection = &TagRange{From: from, To: to}
		}
	}
	monthCats := map[string]float64{}
	selCats := map[string]float64{}
	for d := p.From; d.Before(p.To); d = d.AddDate(0, 0, 1) {
		ds, ok := byDay[d]
		if !ok {
			ds = TagDaySpend{Day: d}
		}
		m.Days = append(m.Days, ds)
		m.Max = max(m.Max, ds.Amount)
		m.Month.Amount += ds.Amount
		sel := m.Selected(d)
		if sel {
			m.Selection.Amount += ds.Amount
		}
		for _, c := range ds.ByCategory {
			monthCats[tagCatID(c)] += c.Amount
			if sel {
				selCats[tagCatID(c)] += c.Amount
			}
		}
	}
	rangeCats := func(r *TagRange, by map[string]float64) {
		r.Amount = s.Currency.Round(r.Amount)
		for _, c := range s.ByCategory {
			if a := by[tagCatID(c)]; a != 0 {
				r.ByCategory = append(r.ByCategory, TagCategorySpend{Category: c.Category, Amount: s.Currency.Round(a)})
			}
		}
		sort.SliceStable(r.ByCategory, func(i, j int) bool { return r.ByCategory[i].Amount > r.ByCategory[j].Amount })
	}
	rangeCats(&m.Month, monthCats)
	if m.Selection != nil {
		rangeCats(m.Selection, selCats)
	}

	months := map[string]*TagMonthOption{}
	for _, d := range s.ByDay {
		dp := p.Start.Of(d.Day)
		o, ok := months[dp.Key()]
		if !ok {
			o = &TagMonthOption{Period: dp}
			months[dp.Key()] = o
		}
		o.Amount += d.Amount
	}
	for _, o := range months {
		o.Amount = s.Currency.Round(o.Amount)
		m.Months = append(m.Months, *o)
	}
	if _, ok := months[p.Key()]; !ok {
		m.Months = append(m.Months, TagMonthOption{Period: p})
	}
	sort.Slice(m.Months, func(i, j int) bool { return m.Months[i].Period.From.After(m.Months[j].Period.From) })
	if len(s.ByDay) > 0 {
		first, last := p.Start.Of(s.ByDay[0].Day), p.Start.Of(s.ByDay[len(s.ByDay)-1].Day)
		if p.From.After(first.From) {
			m.Prev = p.Prev().Key()
		}
		if p.From.Before(last.From) {
			m.Next = p.Next().Key()
		}
	}
	return m
}
