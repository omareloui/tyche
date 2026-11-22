package models

import (
	"cmp"
	"math"
	"slices"
	"time"
)

const (
	// RecapAvgMonths is how many months before the recap's its average
	// covers.
	RecapAvgMonths = 3
	// RecapTop is how many category changes and largest transactions the
	// recap lists.
	RecapTop = 5
	// RecapBannerDays is how many days into a month the dashboard offers the
	// last one's recap.
	RecapBannerDays = 5
)

// MonthRecap sums up one of the user's months against the one before and
// the average of the RecapAvgMonths before it, in Currency at today's rates.
type MonthRecap struct {
	Period Period
	// Today is the user's today: the month is still open (so far) when it's
	// in it.
	Today    time.Time
	Currency Currency
	// PrevMonth and NextMonth are the months (YYYY-MM) to navigate to, if
	// any.
	PrevMonth, NextMonth string

	// Flow is the month's income and spending, Prev the month before's and
	// Avg the average of the AvgMonths before it with data (up to
	// RecapAvgMonths, 0 without history).
	Flow, Prev, Avg MonthFlow
	AvgMonths       int

	// CategoryChanges are the categories whose spending moved the most
	// against the average, the biggest first.
	CategoryChanges []RecapCategoryChange
	// NewMerchants are the merchants first paid in the month.
	NewMerchants *MerchantSpending
	// Largest are its largest expenses, by their analysis amount.
	Largest []RecapTx
	// Budgets is the month's budget summary.
	Budgets *BudgetSummary
	// Goals are the goals money went to (or out of) in the month, and the
	// ones bought then.
	Goals []RecapGoal
	// NetStart and NetEnd are the net worth (the accounts in the balance) at
	// the month's start and end (today's while open).
	NetStart, NetEnd float64

	// NoSpendDays counts the days of the month (so far) without spending,
	// Busiest is the day with the most.
	NoSpendDays int
	Busiest     DayFlow

	Partial bool
}

// IsOpen tells the month isn't over yet.
func (r *MonthRecap) IsOpen() bool {
	return r.Period.Contains(r.Today)
}

// Days counts the month's days, so far when open.
func (r *MonthRecap) Days() int {
	if r.IsOpen() {
		return r.Period.Day(r.Today) + 1
	}
	return r.Period.Days()
}

// NetChange is how much the net worth moved over the month.
func (r *MonthRecap) NetChange() float64 {
	return r.NetEnd - r.NetStart
}

// RecapCategoryChange is a category's spending of the month against its
// average of the months before, in the recap's currency.
type RecapCategoryChange struct {
	// Category is nil for uncategorized spending.
	Category    *Category
	Name, Color string
	Amount, Avg float64
}

func (c RecapCategoryChange) Change() float64 {
	return c.Amount - c.Avg
}

// RecapTx is an expense with its analysis amount in the recap's currency.
type RecapTx struct {
	Tx     Transaction
	Amount float64
}

// RecapGoal is what was put in a goal in the month, in its currency.
type RecapGoal struct {
	Goal      Goal
	Deposited float64
	// Achieved tells it was bought in the month.
	Achieved bool
}

// RecapPeriod is the month a recap of month (YYYY-MM, empty for the last
// closed one) shows, never after today's.
func RecapPeriod(s MonthStart, month string, today time.Time) (Period, bool) {
	cur := s.Of(today)
	if month == "" {
		return cur.Prev(), true
	}
	m, err := time.Parse("2006-01", month)
	if err != nil {
		return Period{}, false
	}
	p := s.For(m)
	if p.From.After(cur.From) {
		return cur, true
	}
	return p, true
}

// RecapAvgPeriods are the months before p the average covers: up to
// RecapAvgMonths, only the ones ending after first (the first day with
// data; none when zero), latest first.
func RecapAvgPeriods(p Period, first time.Time) []Period {
	if first.IsZero() {
		return nil
	}
	var res []Period
	for i, q := 0, p.Prev(); i < RecapAvgMonths && q.To.After(first); i, q = i+1, q.Prev() {
		res = append(res, q)
	}
	return res
}

// NewRecapFlow sums days (the income and spending per day) into p's flow,
// the month before's and the average of avg.
func NewRecapFlow(p Period, avg []Period, days []DayFlow) (cur, prev, mean MonthFlow) {
	cur = MonthFlow{Month: p.Month, Period: p}
	pp := p.Prev()
	prev = MonthFlow{Month: pp.Month, Period: pp}
	for _, d := range days {
		switch {
		case p.Contains(d.Day):
			cur.In += d.In
			cur.Out += d.Out
		case pp.Contains(d.Day):
			prev.In += d.In
			prev.Out += d.Out
		}
		for _, q := range avg {
			if q.Contains(d.Day) {
				mean.In += d.In
				mean.Out += d.Out
			}
		}
	}
	if n := float64(len(avg)); n > 0 {
		mean.In /= n
		mean.Out /= n
	}
	return cur, prev, mean
}

// RecapDays counts p's days without spending up to last (included) and
// finds the one with the most.
func RecapDays(p Period, last time.Time, days []DayFlow) (noSpend int, busiest DayFlow) {
	n := min(p.Day(last)+1, p.Days())
	spent := map[int]bool{}
	for _, d := range days {
		if !p.Contains(d.Day) || p.Day(d.Day) >= n || d.Out <= 0 {
			continue
		}
		spent[p.Day(d.Day)] = true
		if d.Out > busiest.Out {
			busiest = d
		}
	}
	return max(n-len(spent), 0), busiest
}

// NewRecapCategoryChanges compares each category's spending of p with its
// average over avg, from totals (per day, category and currency, covering
// them all), keeping the RecapTop that moved the most.
func NewRecapCategoryChanges(p Period, avg []Period, def Currency, cats []Category, totals []DayCategoryTotal) (res []RecapCategoryChange, partial bool) {
	byID := make(map[string]*Category, len(cats))
	for i := range cats {
		byID[cats[i].ID] = &cats[i]
	}
	idx := map[string]int{}
	for _, t := range totals {
		inAvg := slices.ContainsFunc(avg, func(q Period) bool { return q.Contains(t.Day) })
		if !p.Contains(t.Day) && !inAvg {
			continue
		}
		amount, ok := Convert(t.Amount, &t.Currency, def.String)
		if !ok {
			partial = true
			continue
		}
		i, ok := idx[t.CategoryID]
		if !ok {
			c := RecapCategoryChange{Name: "Uncategorized", Color: "gray"}
			if cat := byID[t.CategoryID]; cat != nil {
				c.Category, c.Name, c.Color = cat, cat.Name, cat.Color
			}
			i = len(res)
			idx[t.CategoryID] = i
			res = append(res, c)
		}
		if p.Contains(t.Day) {
			res[i].Amount += amount
		} else {
			res[i].Avg += amount / float64(len(avg))
		}
	}
	res = slices.DeleteFunc(res, func(c RecapCategoryChange) bool { return math.Abs(c.Change()) < 0.005 })
	slices.SortStableFunc(res, func(a, b RecapCategoryChange) int {
		return cmp.Compare(math.Abs(b.Change()), math.Abs(a.Change()))
	})
	if len(res) > RecapTop {
		res = res[:RecapTop]
	}
	return res, partial
}

// LargestExpenses are the n largest of txs by their analysis amount in def
// (reimbursed expenses net, reimbursements and transfer pair legs out).
func LargestExpenses(txs []Transaction, def Currency, n int) (res []RecapTx, partial bool) {
	for _, t := range txs {
		if t.Kind != ExpenseTxKind {
			continue
		}
		v := t.AnalysisAmount()
		if v <= 0 {
			continue
		}
		amount, ok := Convert(v, &t.Account.Currency, def.String)
		if !ok {
			partial = true
			continue
		}
		res = append(res, RecapTx{Tx: t, Amount: amount})
	}
	slices.SortStableFunc(res, func(a, b RecapTx) int { return cmp.Compare(b.Amount, a.Amount) })
	if len(res) > n {
		res = res[:n]
	}
	return res, partial
}

// RecapNetWorth is the net worth of accs (today's balances) in def at p's
// start and end, worked back through daily (the money each moved per day by
// account id, since p's start at least): the accounts in the balance, like
// the accounts summary's default. A month still open ends today.
func RecapNetWorth(accs []Account, def Currency, daily map[string][]DayFlow, p Period) (start, end float64, partial bool) {
	for i := range accs {
		a := &accs[i]
		if a.IsArchive || a.IsDeleted() || a.ExcludeFromBalance || a.Kind.Enum == AccKindGoal {
			continue
		}
		rate, ok := Convert(1, &a.Currency, def.String)
		if !ok {
			partial = true
			continue
		}
		e, in := a.Amount, 0.0
		for _, d := range daily[a.ID] {
			switch {
			case !d.Day.Before(p.To):
				e -= d.In - d.Out
			case !d.Day.Before(p.From):
				in += d.In - d.Out
			}
		}
		end += e * rate
		start += (e - in) * rate
	}
	return start, end, partial
}

// NewRecapGoals are the goals with money put in (or taken out) in the month,
// from deposited (per goal id, in its currency), and the ones bought in p,
// achieved first then the most put in.
func NewRecapGoals(goals []Goal, deposited map[string]float64, p Period) []RecapGoal {
	var res []RecapGoal
	for _, gl := range goals {
		g := RecapGoal{Goal: gl, Deposited: deposited[gl.ID], Achieved: !gl.FulfilledAt.IsZero() && p.Contains(gl.FulfilledAt)}
		if g.Achieved || math.Abs(g.Deposited) >= 0.005 {
			res = append(res, g)
		}
	}
	slices.SortStableFunc(res, func(a, b RecapGoal) int {
		if c := cmp.Compare(btoi(b.Achieved), btoi(a.Achieved)); c != 0 {
			return c
		}
		return cmp.Compare(b.Deposited, a.Deposited)
	})
	return res
}
