package models_test

import (
	"errors"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/models"
)

func TestFitLoan(t *testing.T) {
	strategies := []models.InstallmentPlan{
		{Strategy: models.StrategyKnownAmount},
		{Strategy: models.StrategyFlat, Rate: 18},
		{Strategy: models.StrategyReducing, Rate: 18},
		{Strategy: models.StrategyDecreasing, Rate: 18},
	}
	for _, plan := range strategies {
		t.Run(plan.Strategy.String(), func(t *testing.T) {
			plan.DownPayment, plan.DownPaymentPct = 20, true
			s, err := models.FitLoan(plan, 1_000_000, 50_000)
			if err != nil {
				t.Fatal(err)
			}
			if s.MaxPayment() > 50_000.005 {
				t.Errorf("max payment %v over the monthly amount", s.MaxPayment())
			}
			// One month less is over the monthly amount.
			shorter := plan
			shorter.Months = len(s.Rows) - 1
			if shorter.Months > 0 {
				if ss, err := shorter.Schedule(1_000_000); err == nil && ss.MaxPayment() <= 50_000 {
					t.Errorf("%d months isn't the shortest, %d fits", len(s.Rows), shorter.Months)
				}
			}
		})
	}

	t.Run("interest-free splits the loan", func(t *testing.T) {
		s, err := models.FitLoan(models.InstallmentPlan{DownPayment: 200_000}, 1_000_000, 50_000)
		if err != nil {
			t.Fatal(err)
		}
		if len(s.Rows) != 16 || !approx(s.FirstPayment(), 50_000, 0.01) {
			t.Errorf("%d × %v", len(s.Rows), s.FirstPayment())
		}
	})

	t.Run("set months are kept", func(t *testing.T) {
		s, err := models.FitLoan(models.InstallmentPlan{Strategy: models.StrategyFlat, Rate: 10, Months: 12}, 120_000, 1_000)
		if err != nil {
			t.Fatal(err)
		}
		if len(s.Rows) != 12 {
			t.Errorf("%d months", len(s.Rows))
		}
	})

	t.Run("too low for the interest", func(t *testing.T) {
		_, err := models.FitLoan(models.InstallmentPlan{Strategy: models.StrategyReducing, Rate: 24}, 1_000_000, 15_000)
		if !errors.Is(err, models.ErrPlanMonthlyTooLow) {
			t.Errorf("err = %v", err)
		}
	})
}

func TestGoalPlanProject(t *testing.T) {
	now := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	nov := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	t.Run("save the price", func(t *testing.T) {
		p := models.GoalPlan{Monthly: 50_000, StartsOn: now}
		pr, err := p.Project(1_000_000, 0, models.MonthStart(1), now)
		if err != nil {
			t.Fatal(err)
		}
		if pr.SaveMonths != 20 || !pr.BuyOn.Equal(nov.AddDate(0, 19, 0)) || pr.Loan != nil || pr.TotalCost != 1_000_000 {
			t.Errorf("%d months, buy on %v", pr.SaveMonths, pr.BuyOn)
		}
		amount, day := pr.NextDeposit()
		if amount != 50_000 || !day.Equal(now) {
			t.Errorf("next %v on %v", amount, day)
		}
	})

	t.Run("this month's deposit is in", func(t *testing.T) {
		p := models.GoalPlan{Monthly: 50_000, StartsOn: now}
		pr, err := p.Project(1_000_000, 50_000, models.MonthStart(1), now)
		if err != nil {
			t.Fatal(err)
		}
		if pr.SaveMonths != 19 || !pr.BuyOn.Equal(nov.AddDate(0, 19, 0)) {
			t.Errorf("%d months, buy on %v", pr.SaveMonths, pr.BuyOn)
		}
		amount, day := pr.NextDeposit()
		if amount != 50_000 || !day.Equal(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("next %v on %v", amount, day)
		}
	})

	t.Run("scheduled deposits are covered", func(t *testing.T) {
		p := models.GoalPlan{Monthly: 1_000, StartsOn: now}
		// 1k this month and 1.5k scheduled: this month is in, and there are
		// fewer deposits left (97.5k: 97 × 1k and 500).
		pr, _ := p.Project(100_000, 2_500, models.MonthStart(1), now)
		amount, day := pr.NextDeposit()
		if amount != 1_000 || !day.Equal(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)) || pr.SaveMonths != 98 {
			t.Errorf("next %v on %v, %d deposits", amount, day, pr.SaveMonths)
		}
	})

	t.Run("down payment and installments", func(t *testing.T) {
		p := models.GoalPlan{
			Mode:     models.GoalPlanInstallments,
			Monthly:  50_000,
			Loan:     &models.InstallmentPlan{Strategy: models.StrategyFlat, Rate: 18, DownPayment: 20, DownPaymentPct: true, AdminFee: 1, AdminFeePct: true},
			StartsOn: now,
		}
		pr, err := p.Project(1_000_000, 0, models.MonthStart(1), now)
		if err != nil {
			t.Fatal(err)
		}
		// 200k down + 8k fee.
		if !approx(pr.Target, 208_000, 0.01) || pr.SaveMonths != 5 || pr.Loan == nil {
			t.Fatalf("target %v, %d months", pr.Target, pr.SaveMonths)
		}
		if pr.Loan.MaxPayment() > 50_000 || !pr.DoneOn.Equal(pr.BuyOn.AddDate(0, len(pr.Loan.Rows), 0)) || pr.Extra <= 0 {
			t.Errorf("loan %v × %d, done %v, extra %v", pr.Loan.MaxPayment(), len(pr.Loan.Rows), pr.DoneOn, pr.Extra)
		}
		amount, _ := pr.NextDeposit()
		if amount != 50_000 {
			t.Errorf("next %v", amount)
		}
		// Ahead: the 18k left next month.
		pr, _ = p.Project(1_000_000, 190_000, models.MonthStart(1), now)
		if rows := pr.Deposits(); len(rows) != 1 || rows[0].Amount != 18_000 || !rows[0].On.Equal(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("last deposits %+v", rows)
		}
	})

	t.Run("deposits", func(t *testing.T) {
		p := models.GoalPlan{Monthly: 1_000, StartsOn: now}
		pr, _ := p.Project(3_500, 400, models.MonthStart(1), now)
		rows := pr.Deposits()
		if len(rows) != 4 || rows[0].Amount != 600 || !rows[0].On.Equal(now) || rows[3].Amount != 500 ||
			!rows[3].On.Equal(time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)) || !approx(rows[3].Saved, 3_500, 0.01) {
			t.Errorf("rows = %+v", rows)
		}
		if !rows[3].On.Equal(pr.BuyOn) {
			t.Errorf("last deposit %v, buy on %v", rows[3].On, pr.BuyOn)
		}
	})

	t.Run("more than the monthly amount leaves fewer deposits", func(t *testing.T) {
		// Started in September, 4 months' worth in already (in any number of
		// deposits, this month or before): this month is covered.
		p := models.GoalPlan{Monthly: 1_000, StartsOn: now.AddDate(0, -2, 0)}
		pr, _ := p.Project(10_000, 4_000, models.MonthStart(1), now)
		amount, day := pr.NextDeposit()
		if amount != 1_000 || !day.Equal(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("next %v on %v", amount, day)
		}
		if pr.SaveMonths != 6 || !pr.BuyOn.Equal(time.Date(2027, 5, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("%d months, buy on %v", pr.SaveMonths, pr.BuyOn)
		}
		// Far ahead, it's only fewer deposits left, from next month.
		pr, _ = p.Project(29_988, 24_500, models.MonthStart(1), now)
		if rows := pr.Deposits(); len(rows) != 6 || !rows[0].On.Equal(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)) || rows[5].Amount != 488 ||
			!pr.BuyOn.Equal(time.Date(2027, 5, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("rows %+v, buy on %v", rows, pr.BuyOn)
		}
		// A part of a month ahead tops it up.
		pr, _ = p.Project(10_000, 3_400, models.MonthStart(1), now)
		if amount, day := pr.NextDeposit(); amount != 600 || !day.Equal(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("next %v on %v", amount, day)
		}
	})

	t.Run("missed months are asked now", func(t *testing.T) {
		p := models.GoalPlan{Monthly: 1_000, StartsOn: now.AddDate(0, -2, 0)}
		pr, _ := p.Project(10_000, 500, models.MonthStart(1), now)
		rows := pr.Deposits()
		if len(rows) < 2 || rows[0].Amount != 2_500 || !rows[0].On.Equal(now) || !rows[1].On.Equal(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("rows = %+v", rows)
		}
	})

	t.Run("on track", func(t *testing.T) {
		p := models.GoalPlan{Monthly: 1_000, StartsOn: now.AddDate(0, -3, 0), StartSaved: 500}
		pr, _ := p.Project(100_000, 2_000, models.MonthStart(1), now)
		if !approx(pr.Due, 3_500, 0.01) || !approx(pr.Behind(), 1_500, 0.01) || pr.Ahead() != 0 {
			t.Errorf("due %v, behind %v, ahead %v", pr.Due, pr.Behind(), pr.Ahead())
		}
		pr, _ = p.Project(100_000, 6_000, models.MonthStart(1), now)
		if pr.Behind() != 0 || !approx(pr.Ahead(), 1_500, 0.01) {
			t.Errorf("behind %v, ahead %v", pr.Behind(), pr.Ahead())
		}
	})

	t.Run("needs a monthly amount", func(t *testing.T) {
		if _, err := (&models.GoalPlan{}).Project(100, 0, models.MonthStart(1), now); !errors.Is(err, models.ErrPlanNoMonthly) {
			t.Errorf("err = %v", err)
		}
	})
}

func TestGoalPlanMoney(t *testing.T) {
	egp := models.Currency{ID: "egp", String: "EGP"}
	now := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	ws := &models.WorkSettings{MonthlySalary: 80_000, Currency: egp, HoursPerWeek: 40}
	cats := []models.Category{
		{BudgetAmount: 20_000, BudgetCurrency: &egp, Kinds: []models.TxKind{models.ExpenseTxKind}},
		{BudgetAmount: 5_000, BudgetCurrency: &egp, Kinds: []models.TxKind{models.IncomeTxKind}},
		{Kinds: []models.TxKind{models.ExpenseTxKind}},
	}
	car := models.Goal{ID: "car", Price: 1_000_000, Currency: egp, Plan: &models.GoalPlan{Monthly: 50_000}}
	goals := []models.Goal{
		car,
		{ID: "phone", Price: 30_000, Currency: egp, Plan: &models.GoalPlan{Monthly: 5_000, StartsOn: now}},
		// Saved already, so it takes nothing.
		{ID: "tv", Price: 1_000, Currency: egp, Account: &models.Account{Amount: 1_000}, Plan: &models.GoalPlan{Monthly: 500}},
		{ID: "trip", Price: 10_000, Currency: egp},
	}

	m := models.NewGoalPlanMoney(&car, ws, cats, goals, models.MonthStart(1), now, now)
	if m.Salary != 80_000 || m.Budgets != 20_000 || m.OtherPlans != 5_000 || m.Free() != 55_000 || !m.IsCurrent(now) {
		t.Errorf("%+v, free %v", m, m.Free())
	}
	// The phone is saved for in 6 months: nothing taken after.
	if m := models.NewGoalPlanMoney(&car, ws, cats, goals, models.MonthStart(1), now, now.AddDate(0, 6, 0)); m.OtherPlans != 0 || m.Free() != 60_000 {
		t.Errorf("6 months on %+v", m)
	}

	// The phone's last deposit is in April: free from May.
	ms := models.PlanMilestones(&car, ws, cats, goals, models.MonthStart(1), now)
	if len(ms) != 1 || !ms[0].On.Equal(time.Date(2027, 5, 1, 0, 0, 0, 0, time.UTC)) || len(ms[0].Done) != 1 || ms[0].Done[0].ID != "phone" || ms[0].Free != 60_000 {
		t.Errorf("milestones = %+v", ms)
	}
	if ms := models.PlanMilestones(&car, ws, cats, goals[2:], models.MonthStart(1), now); len(ms) != 0 {
		t.Errorf("milestones without plans = %+v", ms)
	}

	if m := models.NewGoalPlanMoney(&car, &models.WorkSettings{}, cats, goals, models.MonthStart(1), now, now); m.HasSalary() || m.Free() != 0 {
		t.Errorf("without a salary %+v", m)
	}
}

func TestGoalPlanOutlayBought(t *testing.T) {
	egp := models.Currency{ID: "egp", String: "EGP"}
	bought := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	gl := models.Goal{
		ID: "car", Price: 120_000, Currency: egp, FulfilledAt: bought,
		Plan: &models.GoalPlan{Mode: models.GoalPlanInstallments, Monthly: 10_000, Loan: &models.InstallmentPlan{Months: 12}},
	}
	if o := gl.Plan.Outlay(&gl, bought.AddDate(0, 3, 0)); o != 10_000 {
		t.Errorf("during the loan %v", o)
	}
	if o := gl.Plan.Outlay(&gl, bought.AddDate(0, 13, 0)); o != 0 {
		t.Errorf("after the loan %v", o)
	}
}

func TestPickGoalSource(t *testing.T) {
	egp := models.Currency{ID: "egp", String: "EGP"}
	bank := models.AccountKind{Enum: models.AccKindBankAcc}
	accs := []models.Account{
		{ID: "default", Amount: 1_000, Currency: egp, Kind: bank, IsDefault: true},
		{ID: "savings", Amount: 60_000, Currency: egp, Kind: bank, ExcludeFromBalance: true},
		{ID: "cash", Amount: 5_000, Currency: egp, Kind: models.AccountKind{Enum: models.AccKindCash}},
		{ID: "card", Amount: 100_000, Currency: egp, Kind: models.AccountKind{Enum: models.AccKindCreditCard}},
		{ID: "old", Amount: 100_000, Currency: egp, Kind: bank, IsArchive: true},
		{ID: "owed", Amount: -3_000, Currency: egp, Kind: bank},
	}

	src := models.PickGoalSource(accs, 50_000, &egp)
	if src.Available != 66_000 || src.Account == nil || src.Account.ID != "savings" {
		t.Errorf("%+v", src)
	}
	if src := models.PickGoalSource(accs, 500, &egp); src.Account.ID != "default" {
		t.Errorf("small deposit from %s", src.Account.ID)
	}
	if src := models.PickGoalSource(accs, 100_000, &egp); src.Account.ID != "savings" {
		t.Errorf("too big from %s", src.Account.ID)
	}
	if src := models.PickGoalSource(nil, 1, &egp); src.Account != nil || src.Available != 0 {
		t.Errorf("no accounts %+v", src)
	}
}

func TestGoalPlanSized(t *testing.T) {
	now := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)

	// 9,000 left over 4 months: 2,250 a month.
	p := &models.GoalPlan{Months: 4, StartsOn: now, StartSaved: 3000}
	pr, err := p.Project(12000, 3000, models.MonthStart(1), now)
	if err != nil {
		t.Fatal(err)
	}
	if pr.Plan.Monthly != 2250 || pr.SaveMonths != 4 || !pr.BuyOn.Equal(time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("projection = %+v", pr)
	}

	// Starting later: the first deposit on its day.
	later := time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC)
	p = &models.GoalPlan{Monthly: 3000, StartsOn: later}
	pr, err = p.Project(9000, 0, models.MonthStart(1), now)
	if err != nil {
		t.Fatal(err)
	}
	if d := pr.Deposits(); len(d) != 3 || !d[0].On.Equal(later) || !pr.BuyOn.Equal(time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("deposits = %+v, buy on %v", d, pr.BuyOn)
	}

	// With a loan, only the down payment is saved.
	loan := &models.InstallmentPlan{Months: 10, DownPayment: 20, DownPaymentPct: true}
	p = &models.GoalPlan{Mode: models.GoalPlanInstallments, Loan: loan, Months: 4, StartsOn: now}
	if got := p.Sized(12000).Monthly; got != 600 {
		t.Errorf("monthly = %v, want 600 (2,400 down over 4 months)", got)
	}

	// Saved already: the plan as is.
	p = &models.GoalPlan{Months: 4, StartsOn: now, StartSaved: 12000, Monthly: 500}
	if got := p.Sized(12000).Monthly; got != 500 {
		t.Errorf("monthly = %v, want the stored 500", got)
	}
}

func TestPlanMilestones(t *testing.T) {
	egp := models.Currency{ID: "egp", String: "EGP"}
	now := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	ws := &models.WorkSettings{MonthlySalary: 10_000, Currency: egp, HoursPerWeek: 40}
	goals := []models.Goal{
		// 12,000 at 6,000 a month: November, December.
		{ID: "tv", Name: "TV", Price: 12_000, Currency: egp, Plan: &models.GoalPlan{Monthly: 6_000, StartsOn: now}},
		// 5,000 down at 5,000 a month (November), then 3 payments of 4,000.
		{ID: "car", Name: "Car", Price: 17_000, Currency: egp, Plan: &models.GoalPlan{
			Mode: models.GoalPlanInstallments, Monthly: 5_000, StartsOn: now,
			Loan: &models.InstallmentPlan{Months: 3, DownPayment: 5_000},
		}},
	}
	ms := models.PlanMilestones(&models.Goal{Currency: egp}, ws, nil, goals, models.MonthStart(1), now)
	day := func(y int, m time.Month) time.Time { return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC) }
	// November takes 11,000: nothing free. December: TV 6,000 + the car's
	// first payment 4,000. January: the TV's saved, the car's 4,000 left.
	want := []struct {
		on        time.Time
		done      int
		saved     int
		firstFree bool
		free      float64
	}{
		{day(2026, 12), 0, 1, false, 0},
		{day(2027, 1), 1, 0, true, 6_000},
		{day(2027, 3), 1, 0, false, 10_000},
	}
	if len(ms) != len(want) {
		t.Fatalf("milestones = %+v", ms)
	}
	for i, w := range want {
		m := ms[i]
		if !m.On.Equal(w.on) || len(m.Done) != w.done || len(m.Saved) != w.saved || m.FirstFree != w.firstFree || m.Free != w.free {
			t.Errorf("milestone %d = %+v, want %+v", i, m, w)
		}
	}
}

// TestGoalProjectPlan checks a plan counts its deposits from the first day
// of its first user month, at today's rates.
func TestGoalProjectPlan(t *testing.T) {
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	egp := models.Currency{ID: "egp", String: "EGP"}
	usd := models.Currency{ID: "usd", String: "USD"}
	start := models.MonthStart(26)
	now := day(time.November, 6)

	// 932 USD put in on Nov 3, the plan saved later that day.
	goal := func(deposits ...models.Transaction) *models.Goal {
		holding := models.Account{ID: "gl-usd", Currency: usd}
		gl := &models.Goal{
			ID: "car", Price: 1_000_000, Currency: egp,
			Account:  &models.Account{ID: "gl-egp", Currency: egp},
			Holdings: []models.Account{holding},
			Plan:     &models.GoalPlan{Mode: models.GoalPlanSave, Monthly: 50_000, StartsOn: day(time.November, 3).Add(18 * time.Hour)},
		}
		for _, tx := range deposits {
			if tx.FulfilledAt.IsZero() {
				gl.Transactions = append(gl.Transactions, tx)
				continue
			}
			for _, acc := range append([]*models.Account{gl.Account}, &gl.Holdings[0]) {
				if acc.ID == tx.ToAccount.ID {
					acc.Amount += tx.ToAmount
				}
			}
			gl.Transactions = append(gl.Transactions, tx)
		}
		return gl
	}
	deposit := func(on time.Time, to models.Account, amount float64, fulfilled bool) models.Transaction {
		tx := models.Transaction{
			Account: models.Account{ID: "bank", Currency: to.Currency}, Amount: amount,
			ToAccount: &to, ToAmount: amount, ScheduledTo: on,
		}
		if fulfilled {
			tx.FulfilledAt = on
		}
		return tx
	}
	usdIn := func(on time.Time) models.Transaction {
		return deposit(on, models.Account{ID: "gl-usd", Currency: usd}, 932, true)
	}

	for _, tt := range []struct {
		rate   float64
		amount float64
		on     time.Time
	}{
		// Worth more than the monthly amount: nothing asked until
		// December's (Nov 26), topping November's up to two months.
		{55.83, models.RoundCents(100_000 - 932*55.83), day(time.November, 26)},
		// Worth less: only what November misses, asked today.
		{50, 3_400, now},
	} {
		t.Cleanup(fxrate.SetRates(map[string]map[string]float64{"usd": {"egp": tt.rate}}))
		pr, err := goal(usdIn(day(time.November, 3))).ProjectPlan(start, now)
		if err != nil {
			t.Fatal(err)
		}
		if amount, on := pr.NextDeposit(); amount != tt.amount || !on.Equal(tt.on) || pr.Behind() != 0 || pr.Plan.StartSaved != 0 {
			t.Errorf("at %v: next = %v on %v, want %v on %v; behind %v, baseline %v", tt.rate, amount, on, tt.amount, tt.on, pr.Behind(), pr.Plan.StartSaved)
		}
	}
	pr, _ := (&models.GoalPlan{Monthly: 1, StartsOn: now}).Project(100, 0, start, now)
	if got := pr.MonthOf(day(time.November, 26)); !got.Equal(day(time.December, 1)) {
		t.Errorf("month of Nov 26 = %v, want December", got)
	}

	t.Run("the month before is saved already", func(t *testing.T) {
		t.Cleanup(fxrate.SetRates(map[string]map[string]float64{"usd": {"egp": 50}}))
		pr, _ := goal(usdIn(day(time.October, 20))).ProjectPlan(start, now)
		if amount, on := pr.NextDeposit(); amount != 50_000 || !on.Equal(now) {
			t.Errorf("next = %v on %v, want 50000 now", amount, on)
		}
		if pr.Plan.StartSaved != 46_600 {
			t.Errorf("baseline = %v, want 46600", pr.Plan.StartSaved)
		}
	})

	t.Run("a scheduled deposit counts as made", func(t *testing.T) {
		pr, _ := goal(deposit(day(time.November, 10), models.Account{ID: "gl-egp", Currency: egp}, 50_000, false)).ProjectPlan(start, now)
		if amount, on := pr.NextDeposit(); amount != 50_000 || !on.Equal(day(time.November, 26)) {
			t.Errorf("next = %v on %v, want 50000 on Nov 26", amount, on)
		}
	})

	t.Run("calendar months", func(t *testing.T) {
		pr, _ := goal(deposit(day(time.November, 2), models.Account{ID: "gl-egp", Currency: egp}, 50_000, true)).ProjectPlan(models.MonthStart(1), now)
		if amount, on := pr.NextDeposit(); amount != 50_000 || !on.Equal(day(time.December, 1)) {
			t.Errorf("next = %v on %v, want 50000 on Dec 1", amount, on)
		}
	})
}
