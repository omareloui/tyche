package models

import (
	"errors"
	"math"
	"slices"
	"time"
)

// GoalPlanMode is how a goal is paid for.
type GoalPlanMode uint8

const (
	// GoalPlanSave saves the whole price, then buys it.
	GoalPlanSave GoalPlanMode = iota
	// GoalPlanInstallments saves a down payment (and the fees paid on the
	// day), then pays the rest in installments.
	GoalPlanInstallments
)

func (m GoalPlanMode) String() string {
	return [...]string{"save", "installments"}[m]
}

func GoalPlanModeFromString(str string) GoalPlanMode {
	if str == GoalPlanInstallments.String() {
		return GoalPlanInstallments
	}
	return GoalPlanSave
}

// GoalPlan is how the user saves for a goal: an amount put aside every
// month, until the price (or a loan's down payment) is saved. The
// projection is worked out from the goal's progress, so it follows every
// deposit and withdrawal.
type GoalPlan struct {
	Mode GoalPlanMode `json:"mode"`
	// Monthly is what's put aside every month, in the goal's currency. It
	// also pays the loan's installments once bought.
	Monthly float64 `json:"monthly"`
	// Loan is the installment plan of GoalPlanInstallments, its months
	// fitted to Monthly when 0.
	Loan *InstallmentPlan `json:"loan,omitempty"`
	// StartsOn is when the plan was saved, and StartSaved the goal's
	// progress then, to tell if it's on track.
	StartsOn   time.Time `json:"starts_on"`
	StartSaved float64   `json:"start_saved"`
}

var (
	ErrPlanNoMonthly     = errors.New("the plan needs a monthly amount")
	ErrPlanMonthlyTooLow = errors.New("the monthly amount doesn't cover the loan's payments")
)

// maxLoanMonths is the longest loan FitLoan looks for.
const maxLoanMonths = 600

func (p *GoalPlan) IsInstallments() bool {
	return p != nil && p.Mode == GoalPlanInstallments && p.Loan != nil
}

// LoanSchedule is the loan worked out for the price, nil when saving the
// whole price.
func (p *GoalPlan) LoanSchedule(price float64) (*InstallmentSchedule, error) {
	if !p.IsInstallments() {
		return nil, nil
	}
	return FitLoan(*p.Loan, price, p.Monthly)
}

// FitLoan works the plan out for the price. Without its months (or a known
// amount's monthly amount), it takes the shortest term whose payments are
// at most monthly.
func FitLoan(plan InstallmentPlan, price, monthly float64) (*InstallmentSchedule, error) {
	if plan.Months > 0 || (plan.Strategy == StrategyKnownAmount && plan.Amount > 0) {
		return plan.Schedule(price)
	}
	if monthly <= 0 {
		return nil, ErrPlanNoMonthly
	}

	fits := func(n int) (*InstallmentSchedule, bool, error) {
		plan.Months = n
		s, err := plan.Schedule(price)
		if err != nil {
			return nil, false, err
		}
		return s, s.MaxPayment() <= monthly+0.005, nil
	}

	// The payments only go down as the term grows.
	best, ok, err := fits(maxLoanMonths)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrPlanMonthlyTooLow
	}
	lo, hi := 1, maxLoanMonths
	for lo < hi {
		mid := (lo + hi) / 2
		s, ok, err := fits(mid)
		if err != nil {
			return nil, err
		}
		if ok {
			best, hi = s, mid
		} else {
			lo = mid + 1
		}
	}
	return best, nil
}

// GoalProjection is where a plan leads from the goal's progress.
type GoalProjection struct {
	Plan GoalPlan
	// Target is what's saved before buying: the price, or the loan's down
	// payment and fees paid on the day.
	Target, Saved, Left float64
	// SaveMonths is the deposits still to make, this month's included.
	SaveMonths int
	// BuyOn is the month the target is saved, so when it's bought.
	BuyOn time.Time
	// Loan is the installments, nil when saving the whole price, and
	// DoneOn the month of its last payment (BuyOn without a loan).
	Loan   *InstallmentSchedule
	DoneOn time.Time
	// TotalCost is everything paid for it, and Extra what's over the price.
	TotalCost, Extra float64
	// LoanOverMonthly tells the loan's payments are more than Monthly (its
	// months being set).
	LoanOverMonthly bool
	// Due is what should be saved by the start of this month, by the plan.
	Due float64

	now      time.Time
	deposits []GoalDeposit
}

// Project works the plan out for the goal's price from what's saved (the
// deposits scheduled for later counted in). Everything put in since the plan
// started covers its months in order, so putting in more than the monthly
// amount (in one deposit or many, in a month or ahead) moves the next
// deposits on, and missed months are asked again now.
func (p *GoalPlan) Project(price, saved float64, now time.Time) (*GoalProjection, error) {
	if p.Monthly <= 0 {
		return nil, ErrPlanNoMonthly
	}
	pr := &GoalProjection{Plan: *p, Target: price, Saved: saved, TotalCost: price, now: now}

	loan, err := p.LoanSchedule(price)
	if err != nil {
		return nil, err
	}
	if loan != nil {
		pr.Loan = loan
		pr.Target = loan.PaidOnDay()
		pr.TotalCost = loan.TotalPaid
		pr.Extra = loan.Extra()
		pr.LoanOverMonthly = loan.MaxPayment() > p.Monthly+0.005
	}

	month := monthStart(now)
	pr.Left = math.Max(0, pr.Target-saved)
	pr.deposits = pr.schedule()
	pr.SaveMonths = len(pr.deposits)
	pr.BuyOn = month
	if n := len(pr.deposits); n > 0 {
		pr.BuyOn = monthStart(pr.deposits[n-1].On)
	}

	pr.DoneOn = pr.BuyOn
	if n := len(pr.Loan.rows()); n > 0 {
		pr.DoneOn = pr.BuyOn.AddDate(0, pr.Loan.Rows[n-1].Month, 0)
	}

	pr.Due = math.Min(pr.Target, p.StartSaved+p.Monthly*float64(pr.elapsed()))
	return pr, nil
}

// elapsed is the plan's months gone by, this one not included.
func (pr *GoalProjection) elapsed() int {
	return max(0, monthsBetween(monthStart(pr.Plan.StartsOn), monthStart(pr.now)))
}

func (s *InstallmentSchedule) rows() []InstallmentRow {
	if s == nil {
		return nil
	}
	return s.Rows
}

// Behind is what's missing from the plan's due amount, 0 if on track.
func (pr *GoalProjection) Behind() float64 {
	return math.Max(0, pr.Due-pr.Saved)
}

// Ahead is what's saved over the due amount and this month's deposit.
func (pr *GoalProjection) Ahead() float64 {
	if pr.Left <= 0.005 {
		return 0
	}
	return math.Max(0, pr.Saved-pr.Due-pr.Plan.Monthly)
}

// NextDeposit is the plan's next deposit and its day: today while this
// month (or a missed one) isn't covered, else the 1st of the first month
// that isn't. 0 once the target is saved.
func (pr *GoalProjection) NextDeposit() (float64, time.Time) {
	if len(pr.deposits) == 0 {
		return 0, time.Time{}
	}
	return pr.deposits[0].Amount, pr.deposits[0].On
}

// GoalDeposit is one of a plan's deposits still to make.
type GoalDeposit struct {
	On     time.Time
	Amount float64
	// Saved is the goal's progress after it.
	Saved float64
}

// Deposits are the deposits still to make until the target is saved.
func (pr *GoalProjection) Deposits() []GoalDeposit {
	return pr.deposits
}

// schedule works the deposits out from what's been put in since the plan
// started (paid): what's missing through this month is asked today (missed
// months caught up), then one deposit a month from the next, topping up a
// month partly paid ahead. Paying more than the monthly amount (ahead) only
// leaves fewer deposits: the next ones aren't pushed further out.
func (pr *GoalProjection) schedule() []GoalDeposit {
	m := pr.Plan.Monthly
	start := monthStart(pr.Plan.StartsOn)
	elapsed := pr.elapsed()
	paid := pr.Saved - pr.Plan.StartSaved
	left, saved := pr.Left, pr.Saved

	var rows []GoalDeposit
	add := func(amount float64, day time.Time) bool {
		amount = RoundCents(math.Min(amount, left))
		if amount <= 0 {
			return false
		}
		paid += amount
		left -= amount
		saved += amount
		rows = append(rows, GoalDeposit{On: day, Amount: amount, Saved: saved})
		return true
	}

	if due := float64(elapsed+1)*m - paid; due > 0.005 {
		add(due, pr.now)
	}
	for month := elapsed + 1; left > 0.005 && month <= elapsed+maxLoanMonths*2; month++ {
		amount := float64(month+1)*m - paid
		if amount <= 0.005 || amount > m {
			amount = m
		}
		if !add(amount, start.AddDate(0, month, 0)) {
			break
		}
	}
	return rows
}

// Outlay is what the plan takes from a month's money at now: the monthly
// amount while saving, the loan's payment once bought until its last one,
// nothing otherwise.
func (p *GoalPlan) Outlay(gl *Goal, now time.Time) float64 {
	if p == nil {
		return 0
	}
	loan, err := p.LoanSchedule(gl.Price)
	if err != nil {
		loan = nil
	}
	if gl.FulfilledAt.IsZero() {
		if gl.Progress() < p.target(gl.Price, loan)-0.005 {
			return p.Monthly
		}
		if loan != nil {
			return loan.MaxPayment()
		}
		return 0
	}
	if n := len(loan.rows()); n > 0 && now.Before(gl.FulfilledAt.AddDate(0, loan.Rows[n-1].Month, 0)) {
		return loan.MaxPayment()
	}
	return 0
}

func (p *GoalPlan) target(price float64, loan *InstallmentSchedule) float64 {
	if loan != nil {
		return loan.PaidOnDay()
	}
	return price
}

// GoalPlanMoney is a month's money free for a goal, in its currency.
type GoalPlanMoney struct {
	Currency Currency
	// Salary is 0 when not set.
	Salary float64
	// Budgets are the expense categories' budgets of a month.
	Budgets float64
	// OtherPlans are what the other goals' plans take every month.
	OtherPlans float64
}

// NewGoalPlanMoney is what's left of the salary for gl after the budgets
// and the other goals' plans.
func NewGoalPlanMoney(gl *Goal, ws *WorkSettings, cats []Category, goals []Goal, now time.Time) GoalPlanMoney {
	m := GoalPlanMoney{Currency: gl.Currency}
	if ws.IsSet() {
		m.Salary = convert(ws.MonthlySalary, &ws.Currency, &gl.Currency)
	}
	for _, c := range cats {
		if c.HasBudget() && c.HasKind(ExpenseTxKind) {
			m.Budgets += convert(c.BudgetAmount, c.BudgetCurrency, &gl.Currency)
		}
	}
	for i := range goals {
		other := &goals[i]
		if other.ID != gl.ID && other.Plan != nil {
			m.OtherPlans += convert(other.Plan.Outlay(other, now), &other.Currency, &gl.Currency)
		}
	}
	return m
}

func (m GoalPlanMoney) HasSalary() bool {
	return m.Salary > 0
}

// Free is the salary left after the budgets and the other plans, 0 at
// least.
func (m GoalPlanMoney) Free() float64 {
	return RoundCents(math.Max(0, m.Salary-m.Budgets-m.OtherPlans))
}

// GoalSource is where a goal's next deposit can come from.
type GoalSource struct {
	// Available is the money of every account a goal can take from, in the
	// goal's currency.
	Available float64
	// Account is the one to take it from, nil without any.
	Account *Account
}

// PickGoalSource finds the accounts with money for a goal's deposit of
// amount (in cur): every active account but credit cards, debts, gam'yaas
// and goals, those excluded from the balance included. The default account
// is picked if it has enough, else the one with the most (of the goal's
// currency first).
func PickGoalSource(accs []Account, amount float64, cur *Currency) GoalSource {
	src := GoalSource{}
	var usable []Account
	for _, a := range accs {
		if a.IsArchive || !a.DeletedAt.IsZero() || a.Amount <= 0 {
			continue
		}
		switch a.Kind.Enum {
		case AccKindCreditCard, AccKindDebt, AccKindGamyaa, AccKindGoal:
			continue
		}
		usable = append(usable, a)
		src.Available += convert(a.Amount, &a.Currency, cur)
	}
	if len(usable) == 0 {
		return src
	}

	in := func(a *Account) float64 { return convert(a.Amount, &a.Currency, cur) }
	if i := slices.IndexFunc(usable, func(a Account) bool { return a.IsDefault }); i != -1 && in(&usable[i]) >= amount-0.005 {
		src.Account = &usable[i]
		return src
	}
	slices.SortStableFunc(usable, func(a, b Account) int {
		aOk, bOk := in(&a) >= amount-0.005, in(&b) >= amount-0.005
		aSame, bSame := a.Currency.String == cur.String, b.Currency.String == cur.String
		switch {
		case aOk != bOk:
			return boolOrder(aOk)
		case aOk && aSame != bSame:
			return boolOrder(aSame)
		}
		switch d := in(&b) - in(&a); {
		case d > 0:
			return 1
		case d < 0:
			return -1
		}
		return 0
	})
	src.Account = &usable[0]
	return src
}

// PlannedDeposit is a goal plan's deposit as it'd be made: Amount (in the
// goal's currency) towards the goal, SourceTotal (in its currency) out of
// Source into Target, the goal's account of Source's currency (its own,
// receiving Amount, until it holds that currency).
type PlannedDeposit struct {
	On          time.Time
	Goal        *Goal
	Amount      float64
	Source      *Account
	SourceTotal float64
	Target      *Account
}

// TargetAmount is what Target receives, in its currency.
func (d *PlannedDeposit) TargetAmount() float64 {
	if d.Target.Currency.String == d.Source.Currency.String {
		return d.SourceTotal
	}
	return d.Amount
}

// PlannedDeposits are the deposits the plans of goals (unfulfilled, with a
// plan and an account) ask for until until (exclusive), each from the
// account PickGoalSource picks among accs, scheduled ones counted as made.
// Each is made on the first payday (the first day of the user's month,
// starting on start) on or after the day the plan asks it, its salary in.
func PlannedDeposits(goals []Goal, accs []Account, start MonthStart, now, until time.Time) []PlannedDeposit {
	var res []PlannedDeposit
	for i := range goals {
		gl := &goals[i]
		if gl.Plan == nil || !gl.FulfilledAt.IsZero() || gl.Account == nil {
			continue
		}
		pr, err := gl.Plan.Project(gl.Price, gl.Progress()+gl.ScheduledDeposits(), now)
		if err != nil {
			continue
		}
		for _, d := range pr.Deposits() {
			on := Payday(start, d.On)
			if !on.Before(until) {
				break
			}
			src := PickGoalSource(accs, d.Amount, &gl.Currency)
			if src.Account == nil {
				continue
			}
			target := gl.AccountIn(src.Account.Currency.String)
			if target == nil {
				target = gl.Account
			}
			res = append(res, PlannedDeposit{
				On: on, Goal: gl, Amount: d.Amount,
				Source: src.Account, SourceTotal: convert(d.Amount, &gl.Currency, &src.Account.Currency),
				Target: target,
			})
		}
	}
	return res
}

// Payday is the first day of a user's month (starting on start) on or after
// t's day.
func Payday(start MonthStart, t time.Time) time.Time {
	p := start.Of(t)
	if p.From.Equal(dateOnly(t)) {
		return p.From
	}
	return p.Next().From
}

// boolOrder sorts true first.
func boolOrder(first bool) int {
	if first {
		return -1
	}
	return 1
}

func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// SaveTarget is what's saved before buying the goal: the price, or its
// plan's down payment and the fees paid on the day.
func (gl *Goal) SaveTarget() float64 {
	loan, err := gl.Plan.LoanSchedule(gl.Price)
	if err != nil || loan == nil {
		return gl.Price
	}
	return loan.PaidOnDay()
}
