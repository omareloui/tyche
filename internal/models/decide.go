package models

import (
	"fmt"
	"math"
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

// Purchase is what to check: a price paid cash, or in monthly installments.
type Purchase struct {
	Price             float64
	Currency          Currency
	Installments      int
	InstallmentAmount float64
}

func (p *Purchase) IsInstallments() bool {
	return p.Installments > 0
}

func (p *Purchase) TotalCost() float64 {
	if p.IsInstallments() {
		return p.InstallmentAmount * float64(p.Installments)
	}
	return p.Price
}

// Upfront is what's paid right away: the price, or the first installment.
func (p *Purchase) Upfront() float64 {
	if p.IsInstallments() {
		return p.InstallmentAmount
	}
	return p.Price
}

// GoalImpact is how a purchase affects a goal in progress, in its currency.
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
	Purchase        Purchase
	Currency        Currency
	BalanceNow      float64
	Upfront         float64
	TotalCost       float64
	BalanceAfter    float64
	BalanceAfterAll float64
	// Markup is the installments' extra cost over the cash price.
	Markup float64
	// SalaryShare is the monthly outlay (the installment, or the price when
	// paying cash) as a share of the monthly salary, 0 if not set.
	SalaryShare float64
	WorkHours   float64
	Goals       []GoalImpact
	Verdict     Verdict
	Reasons     []string
}

// GoalPaces are each goal's average monthly contribution in its currency.
type GoalPaces map[string]float64

// CheckAffordability weighs a purchase against the balances and the
// unfulfilled goals.
func CheckAffordability(p Purchase, balances []Balance, goals []Goal, paces GoalPaces, ws *WorkSettings, dflt *Currency) Affordability {
	a := Affordability{Purchase: p, Currency: *dflt}

	for _, b := range balances {
		a.BalanceNow += convert(b.Amount, &b.Currency, dflt)
	}

	a.Upfront = convert(p.Upfront(), &p.Currency, dflt)
	a.TotalCost = convert(p.TotalCost(), &p.Currency, dflt)
	a.BalanceAfter = a.BalanceNow - a.Upfront
	a.BalanceAfterAll = a.BalanceNow - a.TotalCost
	if p.IsInstallments() {
		a.Markup = a.TotalCost - convert(p.Price, &p.Currency, dflt)
	}
	a.SalaryShare = ws.SalaryShare(p.Upfront(), &p.Currency)
	a.WorkHours = ws.HoursFor(p.TotalCost(), &p.Currency)

	maxDelay := 0.0
	for _, gl := range goals {
		rem := gl.Remaining()
		if !gl.FulfilledAt.IsZero() || rem <= 0 {
			continue
		}
		cost := convert(p.TotalCost(), &p.Currency, &gl.Currency)
		gi := GoalImpact{Goal: gl, Remaining: rem, Cost: cost, PctOfRemaining: cost / rem * 100, DelayMonths: -1}
		if pace := paces[gl.ID]; pace > 0 {
			gi.DelayMonths = cost / pace
			maxDelay = max(maxDelay, gi.DelayMonths)
		}
		a.Goals = append(a.Goals, gi)
	}

	switch {
	case a.BalanceAfter < 0:
		a.Verdict = VerdictNo
		a.Reasons = append(a.Reasons, "It's more than your balance")
	case a.BalanceAfterAll < 0:
		a.Verdict = VerdictCareful
		a.Reasons = append(a.Reasons, "All the installments are more than your balance today")
	}
	if a.SalaryShare > 0.3 && a.Verdict == VerdictYes {
		a.Verdict = VerdictCareful
	}
	if a.SalaryShare > 0.3 {
		a.Reasons = append(a.Reasons, fmt.Sprintf("It takes %.0f%% of a month's salary", a.SalaryShare*100))
	}
	if maxDelay > 1 {
		if a.Verdict == VerdictYes {
			a.Verdict = VerdictCareful
		}
		a.Reasons = append(a.Reasons, fmt.Sprintf("It sets a goal back by %.1f months", maxDelay))
	}
	if a.BalanceNow > 0 && a.TotalCost > a.BalanceNow/2 && a.BalanceAfter >= 0 {
		if a.Verdict == VerdictYes {
			a.Verdict = VerdictCareful
		}
		a.Reasons = append(a.Reasons, "It's over half your balance")
	}

	return a
}
