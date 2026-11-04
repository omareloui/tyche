package models

import (
	"cmp"
	"slices"
	"time"
)

// UncategorizedKey stands for the uncategorized spending in the budget
// summary's hidden list.
const UncategorizedKey = "none"

// DayCategoryTotal is a category's spending of a day in a currency.
type DayCategoryTotal struct {
	Day time.Time
	CategoryTotal
}

// BudgetSummaryItem is a category's spending of the month, against its
// budget if any, in the summary's currency.
type BudgetSummaryItem struct {
	// Category is nil for uncategorized spending.
	Category *Category
	Name     string
	Color    string
	Spent    float64
	// Budget is 0 without one.
	Budget float64
	Hidden bool
	// Daily is the spending per day of the month, from the 1st.
	Daily []float64
}

// Key identifies the item in the hidden list.
func (it BudgetSummaryItem) Key() string {
	if it.Category == nil {
		return UncategorizedKey
	}
	return it.Category.ID
}

func (it BudgetSummaryItem) HasBudget() bool {
	return it.Budget > 0
}

func (it BudgetSummaryItem) Pct() float64 {
	if it.Budget == 0 {
		return 0
	}
	return it.Spent / it.Budget * 100
}

func (it BudgetSummaryItem) Left() float64 {
	return it.Budget - it.Spent
}

func (it BudgetSummaryItem) Over() bool {
	return it.HasBudget() && it.Left() < 0
}

// BudgetSummary is a month's spending of the expense categories against
// their budgets, in Currency. Hidden items are left out of every total.
type BudgetSummary struct {
	// Month is the 1st of the month Period is named after.
	Month  time.Time
	Period Period
	Today  time.Time
	// Prev and Next are the months (YYYY-MM) to navigate to, if any.
	Prev, Next string
	Currency   Currency
	// Items are the visible budgeted ones, most used first, then the visible
	// unbudgeted ones, most spent first, then the hidden ones.
	Items   []BudgetSummaryItem
	Partial bool
}

// NewBudgetSummary is month's spending of every expense category with a
// budget or some spending, and of the uncategorized one, in def.
func NewBudgetSummary(p Period, today time.Time, def Currency, cats []Category, totals []DayCategoryTotal, hidden []string) *BudgetSummary {
	bs := &BudgetSummary{Month: p.Month, Period: p, Today: dateOnly(today), Currency: def}
	days := bs.Days()

	idx := map[string]int{}
	add := func(it BudgetSummaryItem) {
		it.Daily = make([]float64, days)
		it.Hidden = slices.Contains(hidden, it.Key())
		idx[it.Key()] = len(bs.Items)
		bs.Items = append(bs.Items, it)
	}
	for i := range cats {
		cat := &cats[i]
		if !cat.HasKind(ExpenseTxKind) {
			continue
		}
		it := BudgetSummaryItem{Category: cat, Name: cat.Name, Color: cat.Color}
		if cat.HasBudget() {
			budget, ok := Convert(cat.BudgetAmount, cat.BudgetCurrency, def.String)
			if !ok {
				bs.Partial = true
			}
			it.Budget = budget
		}
		add(it)
	}

	for _, t := range totals {
		key := t.CategoryID
		if key == "" {
			key = UncategorizedKey
		}
		i, ok := idx[key]
		if !ok {
			if key != UncategorizedKey {
				// Not an expense category (anymore).
				continue
			}
			add(BudgetSummaryItem{Name: "Uncategorized", Color: "gray"})
			i = idx[key]
		}
		amount, ok := Convert(t.Amount, &t.Currency, def.String)
		if !ok {
			bs.Partial = true
			continue
		}
		it := &bs.Items[i]
		it.Spent += amount
		if d := bs.Period.Day(t.Day); d >= 0 && d < days {
			it.Daily[d] += amount
		}
	}

	bs.Items = slices.DeleteFunc(bs.Items, func(it BudgetSummaryItem) bool {
		return !it.HasBudget() && it.Spent <= 0
	})
	slices.SortStableFunc(bs.Items, func(a, b BudgetSummaryItem) int {
		if c := cmp.Compare(btoi(a.Hidden), btoi(b.Hidden)); c != 0 {
			return c
		}
		if c := cmp.Compare(btoi(!a.HasBudget()), btoi(!b.HasBudget())); c != 0 {
			return c
		}
		if a.HasBudget() {
			return cmp.Compare(b.Pct(), a.Pct())
		}
		return cmp.Compare(b.Spent, a.Spent)
	})
	return bs
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Days counts the days of the month.
func (bs *BudgetSummary) Days() int {
	return bs.Period.Days()
}

// IsCurrent tells whether the month is today's.
func (bs *BudgetSummary) IsCurrent() bool {
	return bs.Period.Contains(bs.Today)
}

// Elapsed counts the days of the month passed, today included.
func (bs *BudgetSummary) Elapsed() int {
	switch {
	case bs.IsCurrent():
		return bs.Period.Day(bs.Today) + 1
	case !bs.Period.To.After(bs.Today):
		return bs.Days()
	}
	return 0
}

// MonthPct is how much of the month has passed, today included.
func (bs *BudgetSummary) MonthPct() float64 {
	return float64(bs.Elapsed()) / float64(bs.Days()) * 100
}

// DaysLeft counts the days left in the current month, today included; 0 for
// another month.
func (bs *BudgetSummary) DaysLeft() int {
	if !bs.IsCurrent() {
		return 0
	}
	return bs.Days() - bs.Period.Day(bs.Today)
}

// Visible are the items not hidden.
func (bs *BudgetSummary) Visible() []BudgetSummaryItem {
	return slices.DeleteFunc(slices.Clone(bs.Items), func(it BudgetSummaryItem) bool { return it.Hidden })
}

// Hidden are the hidden items.
func (bs *BudgetSummary) Hidden() []BudgetSummaryItem {
	return slices.DeleteFunc(slices.Clone(bs.Items), func(it BudgetSummaryItem) bool { return !it.Hidden })
}

// Budgeted are the visible items with a budget.
func (bs *BudgetSummary) Budgeted() []BudgetSummaryItem {
	return slices.DeleteFunc(bs.Visible(), func(it BudgetSummaryItem) bool { return !it.HasBudget() })
}

// Unbudgeted are the visible items without a budget.
func (bs *BudgetSummary) Unbudgeted() []BudgetSummaryItem {
	return slices.DeleteFunc(bs.Visible(), func(it BudgetSummaryItem) bool { return it.HasBudget() })
}

// UnbudgetedSpent is the spending of the visible items without a budget.
func (bs *BudgetSummary) UnbudgetedSpent() float64 {
	var total float64
	for _, it := range bs.Unbudgeted() {
		total += it.Spent
	}
	return total
}

// Spent is the spending of the visible items.
func (bs *BudgetSummary) Spent() float64 {
	var total float64
	for _, it := range bs.Visible() {
		total += it.Spent
	}
	return total
}

// Budget is the sum of the visible budgets.
func (bs *BudgetSummary) Budget() float64 {
	var total float64
	for _, it := range bs.Budgeted() {
		total += it.Budget
	}
	return total
}

// BudgetedSpent is the spending of the visible budgeted items, the one
// compared with Budget.
func (bs *BudgetSummary) BudgetedSpent() float64 {
	var total float64
	for _, it := range bs.Budgeted() {
		total += it.Spent
	}
	return total
}

// Pct is how much of the visible budgets is spent.
func (bs *BudgetSummary) Pct() float64 {
	if b := bs.Budget(); b > 0 {
		return bs.BudgetedSpent() / b * 100
	}
	return 0
}

// Left is what's left of the visible budgets, negative when over.
func (bs *BudgetSummary) Left() float64 {
	return bs.Budget() - bs.BudgetedSpent()
}

// Ahead tells whether the budgets are spent faster than the month passes.
func (bs *BudgetSummary) Ahead() bool {
	return bs.Left() >= 0 && bs.Pct() > bs.MonthPct()
}

// ItemAhead tells whether an item's budget is spent faster than the month
// passes.
func (bs *BudgetSummary) ItemAhead(it BudgetSummaryItem) bool {
	return it.HasBudget() && !it.Over() && it.Pct() > bs.MonthPct()
}

// Over counts the visible budgets spent past their amount.
func (bs *BudgetSummary) Over() int {
	var n int
	for _, it := range bs.Budgeted() {
		if it.Over() {
			n++
		}
	}
	return n
}

// SafePerDay is what's left of the visible budgets per day left, only for
// the current month.
func (bs *BudgetSummary) SafePerDay() (float64, bool) {
	if !bs.IsCurrent() {
		return 0, false
	}
	var left float64
	for _, it := range bs.Budgeted() {
		left += max(0, it.Left())
	}
	return left / float64(bs.DaysLeft()), true
}

// Cumulative is the running spending of the visible budgeted items per day
// passed.
func (bs *BudgetSummary) Cumulative() []float64 {
	res := make([]float64, bs.Elapsed())
	var total float64
	budgeted := bs.Budgeted()
	for d := range res {
		for _, it := range budgeted {
			total += it.Daily[d]
		}
		res[d] = total
	}
	return res
}

// PaceAt is the budget spent evenly up to day i (0-based) included.
func (bs *BudgetSummary) PaceAt(i int) float64 {
	return bs.Budget() * float64(i+1) / float64(bs.Days())
}
