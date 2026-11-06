package models_test

import (
	"math"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

var egp = models.Currency{ID: "egp", String: "EGP", Symbol: "E£"}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestWorkSettingsHours(t *testing.T) {
	ws := &models.WorkSettings{MonthlySalary: 52000, Currency: egp, HoursPerWeek: 40}
	// 40 h/week × 52/12 weeks = 173.33 h/month → 300/h.
	if got := ws.HourlyRate(); !near(got, 300) {
		t.Fatalf("HourlyRate() = %v, want 300", got)
	}
	if got := ws.HoursFor(1500, &egp); !near(got, 5) {
		t.Errorf("HoursFor(1500) = %v, want 5", got)
	}
	if got := ws.ViewHoursFor(1500, &egp); got != "5.0 h" {
		t.Errorf("ViewHoursFor(1500) = %q", got)
	}
	if got := ws.ViewHoursFor(4800, &egp); got != "16 h · 2.0 work days" {
		t.Errorf("ViewHoursFor(4800) = %q", got)
	}
	if got := ws.ViewHoursFor(104000, &egp); got != "347 h · 2.0 months" {
		t.Errorf("ViewHoursFor(104000) = %q", got)
	}
	if got := ws.ViewHoursFor(50, &egp); got != "10 min" {
		t.Errorf("ViewHoursFor(50) = %q", got)
	}
	if got := ws.SalaryShare(13000, &egp); !near(got, 0.25) {
		t.Errorf("SalaryShare(13000) = %v, want 0.25", got)
	}

	unset := &models.WorkSettings{Currency: egp, HoursPerWeek: 40}
	if unset.HoursFor(1500, &egp) != 0 || unset.ViewHoursFor(1500, &egp) != "" || unset.SalaryShare(1, &egp) != 0 {
		t.Error("unset salary should price nothing")
	}
}

func TestWishlistItemCooling(t *testing.T) {
	created := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	w := &models.WishlistItem{CoolingDays: 7, CreatedAt: created}

	if got := w.ReadyAt(); !got.Equal(created.AddDate(0, 0, 7)) {
		t.Errorf("ReadyAt() = %v", got)
	}
	now := created.Add(36 * time.Hour)
	if w.IsReady(now) || w.DaysLeft(now) != 6 {
		t.Errorf("after 1.5 days: ready %v, days left %d", w.IsReady(now), w.DaysLeft(now))
	}
	now = created.AddDate(0, 0, 7)
	if !w.IsReady(now) || w.DaysLeft(now) != 0 || w.CoolingProgress(now) != 1 {
		t.Error("should be ready after 7 days")
	}
}

func TestSummarizeWishlist(t *testing.T) {
	items := []models.WishlistItem{
		{Status: models.WishlistWaiting, Price: 100, Currency: egp},
		{Status: models.WishlistBought, Price: 200, Currency: egp},
		{Status: models.WishlistAbandoned, Price: 300, Currency: egp},
		{Status: models.WishlistAbandoned, Price: 999, Plan: &models.InstallmentPlan{Months: 3, Amount: 400}, Currency: egp},
	}
	s := models.SummarizeWishlist(items, &egp)
	if s.Waiting != 1 || s.Bought != 1 || s.Abandoned != 2 || s.Saved != 1500 {
		t.Errorf("stats = %+v", s)
	}
	if got := s.AbandonRate(); !near(got, 200.0/3) {
		t.Errorf("AbandonRate() = %v", got)
	}
}

func goalWithSaved(id string, price, saved float64) models.Goal {
	return models.Goal{
		ID: id, Name: id, Price: price, Currency: egp,
		Account: &models.Account{Amount: saved, Currency: egp, Kind: models.AccountKind{Enum: models.AccKindGoal}},
	}
}

func TestCheckAffordability(t *testing.T) {
	today := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	dec1, jan1 := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	balances := []models.Balance{{Amount: 10000, Currency: egp}}
	ws := &models.WorkSettings{MonthlySalary: 52000, Currency: egp, HoursPerWeek: 40}
	goals := []models.Goal{goalWithSaved("car", 100000, 20000), goalWithSaved("trip", 5000, 5000)}
	ctx := func(ws *models.WorkSettings, budget float64, goals []models.Goal) models.AffordContext {
		c := models.AffordContext{
			Balances: balances, Goals: goals, Paces: models.GoalPaces{"car": 2000}, Work: ws,
			Start: models.MonthStart(1), Today: today, Currency: &egp,
		}
		if budget > 0 {
			c.Categories = []models.Category{{BudgetAmount: budget, BudgetCurrency: &egp, Kinds: []models.TxKind{models.ExpenseTxKind}}}
		}
		return c
	}

	t.Run("cash today", func(t *testing.T) {
		c := ctx(ws, 0, goals)
		c.BudgetLeft = 1000
		a, _ := models.CheckAffordability(models.Purchase{Price: 3000, Currency: egp}, c)
		if a.BalanceNow != 10000 || a.BalanceOn != 9000 || a.BalanceAfter != 6000 || a.PaidOnDay != 3000 || !a.IsToday() {
			t.Errorf("balances = %+v", a)
		}
		if !near(a.WorkHours, 10) {
			t.Errorf("WorkHours = %v, want 10", a.WorkHours)
		}
		// The reached trip goal is skipped.
		if len(a.Goals) != 1 || a.Goals[0].Goal.ID != "car" {
			t.Fatalf("goals = %+v", a.Goals)
		}
		g := a.Goals[0]
		if g.Remaining != 80000 || !near(g.PctOfRemaining, 3.75) || !near(g.DelayMonths, 1.5) {
			t.Errorf("goal impact = %+v", g)
		}
		if a.Verdict != models.VerdictCareful {
			t.Errorf("Verdict = %v, want careful (goal delayed 1.5 months)", a.Verdict)
		}
	})

	t.Run("not today, later", func(t *testing.T) {
		salary := &models.WorkSettings{MonthlySalary: 10000, Currency: egp, HoursPerWeek: 40}
		p := models.Purchase{Price: 20000, Currency: egp}
		a, _ := models.CheckAffordability(p, ctx(salary, 4000, nil))
		if a.Verdict != models.VerdictNo || !a.Earliest.Equal(jan1) {
			t.Errorf("Verdict = %v, Earliest = %v, want no until %v", a.Verdict, a.Earliest, jan1)
		}
		if a.Month == nil || a.Month.Free() != 6000 {
			t.Errorf("Month = %+v, want 6,000 free", a.Month)
		}

		p.On = jan1.AddDate(0, 0, 14)
		a, _ = models.CheckAffordability(p, ctx(salary, 4000, nil))
		if a.Verdict == models.VerdictNo || a.BalanceOn != 22000 || a.Paydays != 2 || !a.Earliest.IsZero() {
			t.Errorf("on %v: %+v", p.On, a)
		}
	})

	planned := func() []models.Goal {
		gl := goalWithSaved("laptop", 12000, 0)
		gl.Plan = &models.GoalPlan{Monthly: 6000, StartsOn: today}
		return []models.Goal{gl}
	}
	salary := &models.WorkSettings{MonthlySalary: 10000, Currency: egp, HoursPerWeek: 40}

	t.Run("after a goal's plan is done", func(t *testing.T) {
		a, _ := models.CheckAffordability(models.Purchase{Price: 9000, Currency: egp}, ctx(salary, 4000, planned()))
		// This month's deposit comes out of the balance, December's out of
		// its salary, then the 6,000 are free.
		if a.GoalsDue != 6000 || a.BalanceOn != 4000 || a.Verdict != models.VerdictNo || !a.Earliest.Equal(jan1) {
			t.Errorf("affordability = %+v", a)
		}
		if len(a.FreeDates) != 1 || !a.FreeDates[0].On.Equal(jan1) || a.FreeDates[0].Loan {
			t.Errorf("FreeDates = %+v", a.FreeDates)
		}
		if a.Month == nil || !a.Month.Payday.Equal(dec1) || a.Month.Free() != 0 || len(a.Month.Goals) != 1 {
			t.Errorf("Month = %+v", a.Month)
		}
		if len(a.Goals) != 0 {
			t.Errorf("planned goals weighed by pace: %+v", a.Goals)
		}
	})

	t.Run("installments short in a month", func(t *testing.T) {
		p := models.Purchase{Price: 12000, Currency: egp, Plan: &models.InstallmentPlan{Months: 12}}
		a, err := models.CheckAffordability(p, ctx(salary, 4000, planned()))
		if err != nil {
			t.Fatal(err)
		}
		// December has nothing free: its payment comes out of the balance.
		if a.ShortMonths != 1 || a.MinBalance != 3000 || a.Verdict != models.VerdictCareful || len(a.Short) != 12 || !a.Short[0] || a.Short[1] {
			t.Errorf("affordability = %+v", a)
		}
		if !near(a.BalanceAfterAll, 4000+11*6000-12000) {
			t.Errorf("BalanceAfterAll = %v", a.BalanceAfterAll)
		}
	})

	t.Run("down payment and a pricey plan", func(t *testing.T) {
		plan := &models.InstallmentPlan{Strategy: models.StrategyFlat, Months: 12, Rate: 30, DownPayment: 2000, AdminFee: 100}
		a, err := models.CheckAffordability(models.Purchase{Price: 6000, Currency: egp, Plan: plan}, ctx(ws, 0, nil))
		if err != nil {
			t.Fatal(err)
		}
		// 4,000 financed at 30% flat: 1,200 interest, 433.33 a month.
		if !near(a.PaidOnDay, 2100) || !near(a.FirstPayment, 5200.0/12) || !near(a.TotalCost, 7300) || !near(a.BalanceAfter, 7900) {
			t.Errorf("affordability = %+v", a)
		}
		if a.Verdict != models.VerdictCareful || a.Schedule.APR < 30 {
			t.Errorf("Verdict = %v, APR = %v", a.Verdict, a.Schedule.APR)
		}
	})

	t.Run("no salary: the balance only", func(t *testing.T) {
		noSalary := &models.WorkSettings{HoursPerWeek: 40}
		p := models.Purchase{Price: 12000, Currency: egp, Plan: &models.InstallmentPlan{Months: 12, Amount: 1100}}
		a, _ := models.CheckAffordability(p, ctx(noSalary, 4000, nil))
		if a.HasSalary || a.Month != nil || !near(a.BalanceAfterAll, -3200) || a.Verdict != models.VerdictNo || !a.Earliest.IsZero() {
			t.Errorf("affordability = %+v", a)
		}

		a, _ = models.CheckAffordability(models.Purchase{Price: 100, Currency: egp}, ctx(noSalary, 0, goals))
		if a.Verdict != models.VerdictYes || a.WorkHours != 0 || a.SalaryShare != 0 || !near(a.Goals[0].DelayMonths, 0.05) {
			t.Errorf("affordability = %+v", a)
		}
	})
}

// With months starting on the 26th, a goal's last deposit (on the 1st) is
// in the month before its plan is done, like the goal plans' money counts
// it: the month after takes nothing.
func TestAffordMonthStart26(t *testing.T) {
	today := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	start := models.MonthStart(26)
	gl := goalWithSaved("tv", 9000, 0)
	gl.Plan = &models.GoalPlan{Monthly: 3000, StartsOn: today}
	goals := []models.Goal{gl}
	ws := &models.WorkSettings{MonthlySalary: 10000, Currency: egp, HoursPerWeek: 40}

	ms := models.PlanMilestones(&models.Goal{Currency: egp}, ws, nil, goals, start, today)
	if len(ms) == 0 || !ms[len(ms)-1].On.Equal(time.Date(2027, 1, 26, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("milestones = %+v", ms)
	}
	done := ms[len(ms)-1].On
	if m := models.NewGoalPlanMoney(&models.Goal{Currency: egp}, ws, nil, goals, start, today, done); m.OtherPlans != 0 {
		t.Errorf("plans' money then = %+v", m)
	}
	a, _ := models.CheckAffordability(models.Purchase{Price: 100, Currency: egp, On: done}, models.AffordContext{
		Balances: []models.Balance{{Amount: 1000, Currency: egp}}, Goals: goals, Work: ws,
		Start: start, Today: today, Currency: &egp,
	})
	if a.Month == nil || !a.Month.Payday.Equal(done) || len(a.Month.Goals) != 0 {
		t.Errorf("month on %v = %+v", done, a.Month)
	}
	if len(a.FreeDates) != 1 || !a.FreeDates[0].On.Equal(done) {
		t.Errorf("FreeDates = %+v, want %v", a.FreeDates, done)
	}
}
