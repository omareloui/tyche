package models

import (
	"slices"
	"time"
)

// affordYears is how far ahead a purchase's earliest day is looked for.
const affordYears = 5

// AffordContext is what a purchase is weighed against, in the default
// currency (Currency) unless said otherwise.
type AffordContext struct {
	Balances []Balance
	// BudgetLeft is what's left of this month's expense budgets, still to
	// be spent out of the balance.
	BudgetLeft float64
	// Categories give the expense budgets of every month to come.
	Categories []Category
	// Goals are the unfulfilled goals and the ones paying off a loan: the
	// planned ones take their deposits and payments from every month's
	// money, the others are weighed by their pace (Paces).
	Goals    []Goal
	Paces    GoalPaces
	Work     *WorkSettings
	Start    MonthStart
	Today    time.Time
	Currency *Currency
}

// GoalOutlay is what a goal's plan takes from a month's money, in the
// default currency: deposits while saving, loan payments once bought.
type GoalOutlay struct {
	Goal   *Goal
	Amount float64
	Loan   bool
}

// MonthMoney is a user's month of money from its payday (the first day of
// the user's month), in the default currency: the salary less the expense
// budgets and what the goals' plans take.
type MonthMoney struct {
	Payday          time.Time
	Salary, Budgets float64
	Goals           []GoalOutlay
}

func (m *MonthMoney) GoalsTotal() float64 {
	var t float64
	for _, g := range m.Goals {
		t += g.Amount
	}
	return t
}

// Free is the salary left after the budgets and the goals' plans, negative
// when they take more.
func (m *MonthMoney) Free() float64 {
	return RoundCents(m.Salary - m.Budgets - m.GoalsTotal())
}

// GoalFreeDate is the payday from which a planned goal stops taking money:
// once saved for (and bought), or once its loan is paid off (Loan).
type GoalFreeDate struct {
	Goal *Goal
	On   time.Time
	Loan bool
}

// datedOutlay is one of a goal plan's deposits or loan payments, in the
// goal's currency.
type datedOutlay struct {
	On     time.Time
	Amount float64
	Loan   bool
}

// planOutlays are a goal plan's deposits still to make and its loan's
// payments, or the payments left of the loan it was bought with.
func planOutlays(gl *Goal, now time.Time) []datedOutlay {
	if gl.Plan == nil {
		return nil
	}
	var res []datedOutlay
	if !gl.FulfilledAt.IsZero() {
		loan, err := gl.Plan.LoanSchedule(gl.Price)
		if err != nil {
			return nil
		}
		for _, r := range loan.rows() {
			if on := gl.FulfilledAt.AddDate(0, r.Month, 0); on.After(now) {
				res = append(res, datedOutlay{On: on, Amount: r.Payment, Loan: true})
			}
		}
		return res
	}
	pr, err := gl.Plan.Project(gl.Price, gl.Progress()+gl.ScheduledDeposits(), now)
	if err != nil {
		return nil
	}
	for _, d := range pr.Deposits() {
		res = append(res, datedOutlay{On: d.On, Amount: d.Amount})
	}
	for _, r := range pr.Loan.rows() {
		res = append(res, datedOutlay{On: pr.BuyOn.AddDate(0, r.Month, 0), Amount: r.Payment, Loan: true})
	}
	return res
}

// nextPayday is the first payday after t.
func (c *AffordContext) nextPayday(t time.Time) time.Time {
	return Payday(c.Start, dateOnly(t).AddDate(0, 0, 1))
}

// timeline is every month's money from the first payday after today
// through until, and what the goals' plans ask by today (dueNow), still to
// come out of the balance. Without a salary there's no month's money to
// count on.
func (c *AffordContext) timeline(until time.Time) (tl []MonthMoney, dueNow float64, free []GoalFreeDate) {
	today := dateOnly(c.Today)
	var budgets float64
	for _, cat := range c.Categories {
		if cat.HasBudget() && cat.HasKind(ExpenseTxKind) {
			budgets += convert(cat.BudgetAmount, cat.BudgetCurrency, c.Currency)
		}
	}

	// Each outlay comes out of the month it's in (from its payday), like
	// the goal plans' month's money, the ones of this month or asked by
	// today out of the balance.
	type goalOutlays struct {
		gl     *Goal
		byDay  map[time.Time]float64
		loan   map[time.Time]bool
		lastOn time.Time
	}
	var gls []goalOutlays
	for i := range c.Goals {
		gl := &c.Goals[i]
		g := goalOutlays{gl: gl, byDay: map[time.Time]float64{}, loan: map[time.Time]bool{}}
		for _, o := range planOutlays(gl, today) {
			amount := convert(o.Amount, &gl.Currency, c.Currency)
			on := c.Start.Of(dateOnly(o.On)).From
			if !dateOnly(o.On).After(today) || !on.After(today) {
				dueNow += amount
				continue
			}
			g.byDay[on] += amount
			g.loan[on] = g.loan[on] || o.Loan
			if on.After(g.lastOn) {
				g.lastOn = on
			}
		}
		if len(g.byDay) > 0 {
			gls = append(gls, g)
		}
	}

	if !c.Work.IsSet() {
		return nil, dueNow, nil
	}
	salary := convert(c.Work.MonthlySalary, &c.Work.Currency, c.Currency)
	for p := c.nextPayday(today); !p.After(until); p = c.Start.Of(p).Next().From {
		m := MonthMoney{Payday: p, Salary: salary, Budgets: budgets}
		for _, g := range gls {
			if amount := g.byDay[p]; amount > 0 {
				m.Goals = append(m.Goals, GoalOutlay{Goal: g.gl, Amount: RoundCents(amount), Loan: g.loan[p]})
			}
		}
		tl = append(tl, m)
	}
	for _, g := range gls {
		if on := c.Start.Of(g.lastOn).Next().From; !on.After(until) {
			free = append(free, GoalFreeDate{Goal: g.gl, On: on, Loan: g.loan[g.lastOn]})
		}
	}
	slices.SortStableFunc(free, func(a, b GoalFreeDate) int { return a.On.Compare(b.On) })
	return tl, dueNow, free
}

// affordRun is a purchase made on a day, its balances worked out over the
// months to come.
type affordRun struct {
	// BalanceOn is the balance on the day, BalanceAfter after paying what's
	// paid then, BalanceAfterAll after the last payment.
	BalanceOn, BalanceAfter, BalanceAfterAll float64
	// FreeUntil is the months' money in by the day, over Paydays paydays.
	FreeUntil float64
	Paydays   int
	// MinBalance is the lowest the balance goes from the day, on MinOn.
	MinBalance float64
	MinOn      time.Time
	// Short tells each payment is more than its month's money, ShortMonths
	// how many are.
	Short       []bool
	ShortMonths int
}

func (r affordRun) fits() bool {
	return r.BalanceAfter >= -0.005 && r.MinBalance >= -0.005
}

// run makes the purchase on on: what's paid on the day out of the balance
// then, then every payment out of the balance with the months' money in.
func run(start float64, tl []MonthMoney, on time.Time, onDay float64, sch *InstallmentSchedule, rate func(float64) float64) affordRun {
	var r affordRun
	bal, i := start, 0
	for ; i < len(tl) && !tl[i].Payday.After(on); i++ {
		bal += tl[i].Free()
		r.FreeUntil += tl[i].Free()
		r.Paydays++
	}
	r.BalanceOn = bal
	bal -= onDay
	r.BalanceAfter, r.MinBalance, r.MinOn = bal, bal, on

	for _, row := range sch.rows() {
		day := on.AddDate(0, row.Month, 0)
		for ; i < len(tl) && !tl[i].Payday.After(day); i++ {
			bal += tl[i].Free()
		}
		free := 0.0
		if i > 0 && !tl[i-1].Payday.After(day) {
			free = tl[i-1].Free()
		}
		pmt := rate(row.Payment)
		short := pmt > free+0.005
		r.Short = append(r.Short, short)
		if short {
			r.ShortMonths++
		}
		bal -= pmt
		if bal < r.MinBalance {
			r.MinBalance, r.MinOn = bal, day
		}
	}
	r.BalanceAfterAll = bal
	return r
}

// Spendable is the money free now: the balance less this month's budgets
// left and the goal deposits asked by today.
func (c *AffordContext) Spendable() float64 {
	var bal float64
	for _, b := range c.Balances {
		bal += convert(b.Amount, &b.Currency, c.Currency)
	}
	_, dueNow, _ := c.timeline(dateOnly(c.Today))
	return RoundCents(bal - c.BudgetLeft - dueNow)
}
