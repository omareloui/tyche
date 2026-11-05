package models

import (
	"cmp"
	"slices"
	"time"
)

// FlowMonths is how many months, the current one included, the dashboard's
// income vs expenses chart shows.
const FlowMonths = 6

// MaxCategorySpends is how many categories the spending chart lists before
// folding the rest into "Other".
const MaxCategorySpends = 6

// DayFlow is a day's fulfilled income and spending in the default currency.
type DayFlow struct {
	Day     time.Time
	In, Out float64
}

// MonthFlow is a month's fulfilled income and spending in the default
// currency.
type MonthFlow struct {
	// Month is the 1st of the month Period is named after.
	Month   time.Time
	Period  Period
	In, Out float64
}

func (m MonthFlow) Saved() float64 {
	return m.In - m.Out
}

// SavingsRate is the share of the income left after spending, false without
// income.
func (m MonthFlow) SavingsRate() (float64, bool) {
	if m.In <= 0 {
		return 0, false
	}
	return m.Saved() / m.In * 100, true
}

// Flow is the income and spending of the last FlowMonths months, in the
// default currency.
type Flow struct {
	Today time.Time
	// Days are the days with income or spending, oldest first.
	Days []DayFlow
	// Months are oldest first, the current month last, empty ones included.
	Months []MonthFlow
	// Partial is set when a currency had no rate and was left out.
	Partial bool
}

// FlowFrom is the first day the flow covers.
func FlowFrom(s MonthStart, today time.Time) time.Time {
	cur := s.Of(today)
	return s.For(cur.Month.AddDate(0, -(FlowMonths - 1), 0)).From
}

// NewFlow sums days, oldest first, into the months (starting on s) up to
// today's.
func NewFlow(s MonthStart, today time.Time, days []DayFlow, partial bool) *Flow {
	f := &Flow{Today: dateOnly(today), Days: days, Partial: partial}
	first := s.For(s.Of(today).Month.AddDate(0, -(FlowMonths - 1), 0))
	f.Months = make([]MonthFlow, FlowMonths)
	for i, p := 0, first; i < FlowMonths; i, p = i+1, p.Next() {
		f.Months[i] = MonthFlow{Month: p.Month, Period: p}
	}
	for _, d := range days {
		i := monthsBetween(first.Month, s.Of(d.Day).Month)
		if i < 0 || i >= FlowMonths {
			continue
		}
		f.Months[i].In += d.In
		f.Months[i].Out += d.Out
	}
	return f
}

func (f *Flow) Current() MonthFlow {
	return f.Months[len(f.Months)-1]
}

func (f *Flow) Prev() MonthFlow {
	return f.Months[len(f.Months)-2]
}

// Elapsed counts the days of the current month passed, today included.
func (f *Flow) Elapsed() int {
	return f.Current().Period.Day(f.Today) + 1
}

// PrevToDate is last month up to as many days as passed of this one (or its
// end), to compare a month in progress fairly.
func (f *Flow) PrevToDate() MonthFlow {
	prev := f.Prev()
	m := MonthFlow{Month: prev.Month, Period: prev.Period}
	last := f.Elapsed() - 1
	for _, d := range f.Days {
		if prev.Period.Contains(d.Day) && prev.Period.Day(d.Day) <= last {
			m.In += d.In
			m.Out += d.Out
		}
	}
	return m
}

// Pace is the cumulative spending per day of the current month up to today
// and of all of last month, one value per day starting from the first.
func (f *Flow) Pace() (cur, prev []float64) {
	curP, prevP := f.Current().Period, f.Prev().Period
	cur = make([]float64, f.Elapsed())
	prev = make([]float64, prevP.Days())
	for _, d := range f.Days {
		switch {
		case curP.Contains(d.Day):
			if i := curP.Day(d.Day); i < len(cur) {
				cur[i] += d.Out
			}
		case prevP.Contains(d.Day):
			prev[prevP.Day(d.Day)] += d.Out
		}
	}
	for i := 1; i < len(cur); i++ {
		cur[i] += cur[i-1]
	}
	for i := 1; i < len(prev); i++ {
		prev[i] += prev[i-1]
	}
	return cur, prev
}

// AxisMax is the top of a chart's y axis for values up to max, split in
// ticks readable steps.
func AxisMax(max float64, ticks int) float64 {
	return niceStep(max/float64(ticks)) * float64(ticks)
}

// AxisTicks are the y axis labels from top to bottom, excluding zero.
func AxisTicks(axisMax float64, ticks int) []float64 {
	res := make([]float64, ticks)
	for i := range res {
		res[i] = axisMax * float64(ticks-i) / float64(ticks)
	}
	return res
}

// PctChange is how much cur changed from prev in percent, false when prev
// is 0.
func PctChange(cur, prev float64) (float64, bool) {
	if prev == 0 {
		return 0, false
	}
	return (cur - prev) / prev * 100, true
}

// CategoryTotal is a category's spending in one currency.
type CategoryTotal struct {
	// CategoryID is empty for uncategorized spending.
	CategoryID string
	Currency   Currency
	Amount     float64
}

type CategorySpend struct {
	// Category is nil for uncategorized spending, or "Other".
	Category *Category
	// Uncategorized tells the uncategorized spending from "Other".
	Uncategorized bool
	Name          string
	Color         string
	Amount        float64
}

// CategorySpending is a month's spending per category, in Currency, most
// first.
type CategorySpending struct {
	Month  time.Time
	Period Period
	// Prev and Next are the months (YYYY-MM) to navigate to, if any.
	Prev, Next string
	Currency   Currency
	Items      []CategorySpend
	Total      float64
	Partial    bool
}

// NewCategorySpending merges totals into def per category, keeping the top
// MaxCategorySpends and folding the rest into "Other".
func NewCategorySpending(p Period, def Currency, cats []Category, totals []CategoryTotal) *CategorySpending {
	cs := &CategorySpending{Month: p.Month, Period: p, Currency: def}
	byID := make(map[string]*Category, len(cats))
	for i := range cats {
		byID[cats[i].ID] = &cats[i]
	}

	idx := map[string]int{}
	for _, t := range totals {
		amount, ok := Convert(t.Amount, &t.Currency, def.String)
		if !ok {
			cs.Partial = true
			continue
		}
		if i, ok := idx[t.CategoryID]; ok {
			cs.Items[i].Amount += amount
			continue
		}
		it := CategorySpend{Name: "Uncategorized", Color: "gray", Amount: amount, Uncategorized: true}
		if cat := byID[t.CategoryID]; cat != nil {
			it.Category, it.Name, it.Color, it.Uncategorized = cat, cat.Name, cat.Color, false
		}
		idx[t.CategoryID] = len(cs.Items)
		cs.Items = append(cs.Items, it)
	}

	cs.Items = slices.DeleteFunc(cs.Items, func(it CategorySpend) bool { return it.Amount <= 0 })
	slices.SortStableFunc(cs.Items, func(a, b CategorySpend) int { return cmp.Compare(b.Amount, a.Amount) })
	for _, it := range cs.Items {
		cs.Total += it.Amount
	}
	if len(cs.Items) > MaxCategorySpends {
		other := CategorySpend{Name: "Other", Color: "gray"}
		for _, it := range cs.Items[MaxCategorySpends-1:] {
			other.Amount += it.Amount
		}
		cs.Items = append(cs.Items[:MaxCategorySpends-1], other)
	}
	return cs
}

// Pct is the share of the month's spending.
func (cs *CategorySpending) Pct(amount float64) float64 {
	if cs.Total == 0 {
		return 0
	}
	return amount / cs.Total * 100
}

// Max is the largest item, the full width of the bars.
func (cs *CategorySpending) Max() float64 {
	if len(cs.Items) == 0 {
		return 0
	}
	return slices.MaxFunc(cs.Items, func(a, b CategorySpend) int { return cmp.Compare(a.Amount, b.Amount) }).Amount
}

// BudgetUsage is how much of a category's monthly budget is spent.
type BudgetUsage struct {
	Category Category
	// Spent is in the budget's currency.
	Spent float64
}

func (b BudgetUsage) Pct() float64 {
	return b.Spent / b.Category.BudgetAmount * 100
}

func (b BudgetUsage) Left() float64 {
	return b.Category.BudgetAmount - b.Spent
}

// Budgets is the current month's spending of the categories with a budget.
type Budgets struct {
	Today    time.Time
	Period   Period
	Currency Currency
	Items    []BudgetUsage
	Partial  bool
}

// NewBudgets is the usage of every budgeted category, most used first.
func NewBudgets(s MonthStart, today time.Time, def Currency, cats []Category, totals []CategoryTotal) *Budgets {
	b := &Budgets{Today: dateOnly(today), Period: s.Of(today), Currency: def}
	idx := map[string]int{}
	for _, cat := range cats {
		if !cat.HasBudget() || !cat.HasKind(ExpenseTxKind) {
			continue
		}
		idx[cat.ID] = len(b.Items)
		b.Items = append(b.Items, BudgetUsage{Category: cat})
	}
	for _, t := range totals {
		i, ok := idx[t.CategoryID]
		if !ok {
			continue
		}
		it := &b.Items[i]
		amount, ok := Convert(t.Amount, &t.Currency, it.Category.BudgetCurrency.String)
		if !ok {
			b.Partial = true
			continue
		}
		it.Spent += amount
	}
	slices.SortStableFunc(b.Items, func(x, y BudgetUsage) int { return cmp.Compare(y.Pct(), x.Pct()) })
	return b
}

// MonthPct is how much of the month has passed, today included.
func (b *Budgets) MonthPct() float64 {
	return float64(b.Period.Day(b.Today)+1) / float64(b.Period.Days()) * 100
}

// DaysLeft counts the days left in the month, today included.
func (b *Budgets) DaysLeft() int {
	return b.Period.Days() - b.Period.Day(b.Today)
}

// SafePerDay is what's left of all the budgets, in Currency, per day left;
// false when a rate is missing.
func (b *Budgets) SafePerDay() (float64, bool) {
	var left float64
	for _, it := range b.Items {
		amount, ok := Convert(max(0, it.Left()), it.Category.BudgetCurrency, b.Currency.String)
		if !ok {
			return 0, false
		}
		left += amount
	}
	return left / float64(b.DaysLeft()), true
}

// Over counts the budgets spent past their amount.
func (b *Budgets) Over() int {
	var n int
	for _, it := range b.Items {
		if it.Spent > it.Category.BudgetAmount {
			n++
		}
	}
	return n
}

// Convert is amount of from in to, false without a rate.
func Convert(amount float64, from *Currency, to string) (float64, bool) {
	if from == nil || from.String == to || to == "" || amount == 0 {
		return amount, true
	}
	rate := from.FxRate(to)
	if rate == 0 {
		return 0, false
	}
	return amount * rate, true
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// MonthOf is the first day of t's month, in UTC.
func MonthOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func monthsBetween(from, t time.Time) int {
	return (t.Year()-from.Year())*12 + int(t.Month()) - int(from.Month())
}

func daysIn(t time.Time) int {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
