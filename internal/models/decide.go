package models

import (
	"fmt"
	"math"
	"time"
)

// WorkSettings is what a month of work earns, to price things in work hours.
type WorkSettings struct {
	// MonthlySalary is 0 when not set.
	MonthlySalary float64 `validate:"gte=0"`
	Currency      Currency
	HoursPerWeek  float64 `validate:"gt=0,lte=168"`
}

const weeksPerMonth = 52.0 / 12

func (ws *WorkSettings) IsSet() bool {
	return ws != nil && ws.MonthlySalary > 0 && ws.HoursPerWeek > 0
}

// HourlyRate is what an hour of work earns, in the salary's currency.
func (ws *WorkSettings) HourlyRate() float64 {
	if !ws.IsSet() {
		return 0
	}
	return ws.MonthlySalary / (ws.HoursPerWeek * weeksPerMonth)
}

// HoursFor is how many hours of work pay for amount, 0 if the salary isn't
// set or the rate is unknown.
func (ws *WorkSettings) HoursFor(amount float64, cur *Currency) float64 {
	rate := ws.HourlyRate()
	if rate == 0 {
		return 0
	}
	return convert(amount, cur, &ws.Currency) / rate
}

// ViewHoursFor is HoursFor as text, e.g. "5.5 h" or "20 h · 2.5 work days"
// (of 5 work days a week), "" when unknown.
func (ws *WorkSettings) ViewHoursFor(amount float64, cur *Currency) string {
	h := ws.HoursFor(amount, cur)
	if h <= 0 {
		return ""
	}
	return ws.ViewHours(h)
}

func (ws *WorkSettings) ViewHours(h float64) string {
	if h < 1 {
		return fmt.Sprintf("%.0f min", math.Max(1, math.Round(h*60)))
	}

	hours := fmt.Sprintf("%.0f h", h)
	if math.Round(h*10) < 100 {
		hours = fmt.Sprintf("%.1f h", h)
	}

	day := ws.HoursPerWeek / 5
	month := ws.HoursPerWeek * weeksPerMonth
	switch {
	case h < day:
		return hours
	case h < month:
		return fmt.Sprintf("%s · %.1f work days", hours, h/day)
	}
	return fmt.Sprintf("%s · %.1f months", hours, h/month)
}

// SalaryShare is amount as a share of the monthly salary (1 = a whole
// month), 0 if the salary isn't set.
func (ws *WorkSettings) SalaryShare(amount float64, cur *Currency) float64 {
	if !ws.IsSet() {
		return 0
	}
	return convert(amount, cur, &ws.Currency) / ws.MonthlySalary
}

type Verdict uint8

const (
	VerdictYes Verdict = iota
	VerdictCareful
	VerdictNo
)

func (v Verdict) String() string {
	return [...]string{"You can afford it", "Think it over", "You can't afford it now"}[v]
}

// Purchase is what to check: a price paid cash, or by an installment plan,
// on a day.
type Purchase struct {
	Price    float64
	Currency Currency
	// Plan is nil when paying cash.
	Plan *InstallmentPlan
	// On is the day it's bought, a plan's down payment paid and its months
	// counted from; zero for today.
	On time.Time
}

func (p *Purchase) IsInstallments() bool {
	return p.Plan != nil
}

// Schedule is the plan worked out for the price, nil when paying cash.
func (p *Purchase) Schedule() (*InstallmentSchedule, error) {
	if p.Plan == nil {
		return nil, nil
	}
	return p.Plan.Schedule(p.Price)
}

// GoalImpact is how a purchase affects a goal in progress with no plan, in
// its currency.
type GoalImpact struct {
	Goal      Goal
	Remaining float64
	// Cost is the purchase's total cost in the goal's currency.
	Cost float64
	// PctOfRemaining is the cost as a share of what's left to save, 0 to 100+.
	PctOfRemaining float64
	// DelayMonths is how much later the goal is reached when the cost comes
	// out of its savings at the recent monthly pace, -1 with no recent
	// contributions.
	DelayMonths float64
}

// Affordability is a purchase's impact on the balance and goals, in the
// default currency unless said otherwise.
type Affordability struct {
	Purchase Purchase
	Currency Currency
	// On is the day it's bought (today at the earliest), Today today.
	On, Today time.Time
	// Schedule is the installments in the purchase's currency, nil for cash.
	Schedule   *InstallmentSchedule
	BalanceNow float64
	// BudgetLeft is this month's budgets left to spend and GoalsDue the goals'
	// deposits asked by today, both still to come out of the balance.
	BudgetLeft, GoalsDue float64
	// HasSalary tells if the months' money (salary, less budgets and goal
	// plans) is counted on; without it everything comes out of the balance.
	HasSalary bool
	// FreeUntil is the months' money in from Paydays paydays by On.
	FreeUntil float64
	Paydays   int
	// BalanceOn is the balance on the day.
	BalanceOn float64
	// PaidOnDay is the price, or a plan's down payment and fees on the day.
	PaidOnDay float64
	// FirstPayment is a plan's first monthly payment.
	FirstPayment float64
	TotalCost    float64
	// BalanceAfter is the balance after paying on the day, BalanceAfterAll
	// after the last payment, and MinBalance the lowest it goes in between,
	// on MinOn.
	BalanceAfter    float64
	BalanceAfterAll float64
	MinBalance      float64
	MinOn           time.Time
	// Short tells each payment is more than its month's money, ShortMonths
	// how many are.
	Short       []bool
	ShortMonths int
	// Month is the money of the month On is in (its first one to come
	// before the first payday), nil without a salary.
	Month *MonthMoney
	// FreeDates are when planned goals stop taking money every month.
	FreeDates []GoalFreeDate
	// Earliest is the first payday it fits, when it doesn't on On; zero
	// otherwise or when none does within affordYears.
	Earliest time.Time
	// Extra is what the plan costs over the cash price.
	Extra float64
	// MonthlyOutlay is the highest monthly payment, or the price when paying
	// cash, and SalaryShare it as a share of the monthly salary (0 if not set).
	MonthlyOutlay float64
	SalaryShare   float64
	WorkHours     float64
	Goals         []GoalImpact
	Verdict       Verdict
	Reasons       []string
}

// IsToday tells if it's bought today.
func (a *Affordability) IsToday() bool {
	return a.On.Equal(a.Today)
}

// GoalPaces are each goal's average monthly contribution in its currency.
type GoalPaces map[string]float64

// CheckAffordability weighs a purchase made on its day against the balances,
// the months' money until then and while paying it (salary less budgets
// and goal plans), and the goals with no plan.
func CheckAffordability(p Purchase, c AffordContext) (Affordability, error) {
	dflt := c.Currency
	today := dateOnly(c.Today)
	on := dateOnly(p.On)
	if p.On.IsZero() || on.Before(today) {
		on = today
	}
	a := Affordability{Purchase: p, Currency: *dflt, On: on, Today: today, BudgetLeft: c.BudgetLeft, HasSalary: c.Work.IsSet()}

	for _, b := range c.Balances {
		a.BalanceNow += convert(b.Amount, &b.Currency, dflt)
	}

	sch, err := p.Schedule()
	if err != nil {
		return a, err
	}
	a.Schedule = sch

	total, onDay, outlay := p.Price, p.Price, p.Price
	months := 0
	if sch != nil {
		total, onDay, outlay = sch.TotalPaid, sch.PaidOnDay(), sch.MaxPayment()
		a.FirstPayment = convert(sch.FirstPayment(), &p.Currency, dflt)
		a.Extra = convert(sch.Extra(), &p.Currency, dflt)
		if n := len(sch.Rows); n > 0 {
			months = sch.Rows[n-1].Month
		}
	}
	a.TotalCost = convert(total, &p.Currency, dflt)
	a.PaidOnDay = convert(onDay, &p.Currency, dflt)
	a.MonthlyOutlay = convert(outlay, &p.Currency, dflt)
	a.SalaryShare = c.Work.SalaryShare(outlay, &p.Currency)
	a.WorkHours = c.Work.HoursFor(total, &p.Currency)

	until := today.AddDate(affordYears, 0, 0)
	if on.After(until) {
		until = on
	}
	until = until.AddDate(0, months+1, 0)
	tl, dueNow, freeDates := c.timeline(until)
	a.GoalsDue, a.FreeDates = dueNow, freeDates
	start := a.BalanceNow - a.BudgetLeft - a.GoalsDue
	rate := func(v float64) float64 { return convert(v, &p.Currency, dflt) }

	r := run(start, tl, on, a.PaidOnDay, sch, rate)
	a.BalanceOn, a.BalanceAfter, a.BalanceAfterAll = r.BalanceOn, r.BalanceAfter, r.BalanceAfterAll
	a.FreeUntil, a.Paydays = r.FreeUntil, r.Paydays
	a.MinBalance, a.MinOn = r.MinBalance, r.MinOn
	a.Short, a.ShortMonths = r.Short, r.ShortMonths

	if len(tl) > 0 {
		i := max(0, r.Paydays-1)
		a.Month = &tl[i]
	}

	if !r.fits() {
		for _, m := range tl {
			if m.Payday.After(on) && run(start, tl, m.Payday, a.PaidOnDay, sch, rate).fits() {
				a.Earliest = m.Payday
				break
			}
		}
	}

	maxDelay := 0.0
	for _, gl := range c.Goals {
		rem := gl.Remaining()
		if gl.Plan != nil || !gl.FulfilledAt.IsZero() || rem <= 0 {
			continue
		}
		cost := convert(total, &p.Currency, &gl.Currency)
		gi := GoalImpact{Goal: gl, Remaining: rem, Cost: cost, PctOfRemaining: cost / rem * 100, DelayMonths: -1}
		if pace := c.Paces[gl.ID]; pace > 0 {
			gi.DelayMonths = cost / pace
			maxDelay = max(maxDelay, gi.DelayMonths)
		}
		a.Goals = append(a.Goals, gi)
	}

	careful := func(reason string) {
		if a.Verdict == VerdictYes {
			a.Verdict = VerdictCareful
		}
		a.Reasons = append(a.Reasons, reason)
	}
	switch {
	case a.BalanceAfter < -0.005 && a.IsToday():
		a.Verdict = VerdictNo
		a.Reasons = append(a.Reasons, "It's more than what's left of your balance after this month's budgets")
	case a.BalanceAfter < -0.005:
		a.Verdict = VerdictNo
		a.Reasons = append(a.Reasons, "It's more than you'll have by then")
	case a.MinBalance < -0.005:
		a.Verdict = VerdictNo
		a.Reasons = append(a.Reasons, fmt.Sprintf("The payments run your balance out by %s", a.MinOn.Format("Jan 2006")))
	}
	if !a.HasSalary {
		a.Short = nil
	}
	if a.HasSalary && a.ShortMonths > 0 {
		careful(fmt.Sprintf("%d of the payments are more than what's free in their month", a.ShortMonths))
	}
	if a.SalaryShare > 0.3 {
		careful(fmt.Sprintf("It takes %.0f%% of a month's salary", a.SalaryShare*100))
	}
	if maxDelay > 1 {
		careful(fmt.Sprintf("It sets a goal back by %.1f months", maxDelay))
	}
	if a.BalanceOn > 0 && a.PaidOnDay > a.BalanceOn/2 && a.BalanceAfter >= 0 {
		careful("It's over half of what you'll have then")
	}
	if sch != nil && sch.APR > 30 {
		careful(fmt.Sprintf("The plan costs %s a year in effect", ViewRate(sch.APR)))
	}

	return a, nil
}

// ViewRate is a yearly rate as text, e.g. "18.4%".
func ViewRate(pct float64) string {
	if math.IsInf(pct, 1) {
		return "over 100%"
	}
	return fmt.Sprintf("%.1f%%", pct)
}
