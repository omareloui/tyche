package models_test

import (
	"errors"
	"testing"
	"time"

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
		pr, err := p.Project(1_000_000, 0, now)
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
		pr, err := p.Project(1_000_000, 50_000, now)
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
		pr, _ := p.Project(100_000, 2_500, now)
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
		pr, err := p.Project(1_000_000, 0, now)
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
		pr, _ = p.Project(1_000_000, 190_000, now)
		if rows := pr.Deposits(); len(rows) != 1 || rows[0].Amount != 18_000 || !rows[0].On.Equal(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("last deposits %+v", rows)
		}
	})

	t.Run("deposits", func(t *testing.T) {
		p := models.GoalPlan{Monthly: 1_000, StartsOn: now}
		pr, _ := p.Project(3_500, 400, now)
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
		pr, _ := p.Project(10_000, 4_000, now)
		amount, day := pr.NextDeposit()
		if amount != 1_000 || !day.Equal(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("next %v on %v", amount, day)
		}
		if pr.SaveMonths != 6 || !pr.BuyOn.Equal(time.Date(2027, 5, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("%d months, buy on %v", pr.SaveMonths, pr.BuyOn)
		}
		// Far ahead, it's only fewer deposits left, from next month.
		pr, _ = p.Project(29_988, 24_500, now)
		if rows := pr.Deposits(); len(rows) != 6 || !rows[0].On.Equal(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)) || rows[5].Amount != 488 ||
			!pr.BuyOn.Equal(time.Date(2027, 5, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("rows %+v, buy on %v", rows, pr.BuyOn)
		}
		// A part of a month ahead tops it up.
		pr, _ = p.Project(10_000, 3_400, now)
		if amount, day := pr.NextDeposit(); amount != 600 || !day.Equal(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("next %v on %v", amount, day)
		}
	})

	t.Run("missed months are asked now", func(t *testing.T) {
		p := models.GoalPlan{Monthly: 1_000, StartsOn: now.AddDate(0, -2, 0)}
		pr, _ := p.Project(10_000, 500, now)
		rows := pr.Deposits()
		if len(rows) < 2 || rows[0].Amount != 2_500 || !rows[0].On.Equal(now) || !rows[1].On.Equal(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("rows = %+v", rows)
		}
	})

	t.Run("on track", func(t *testing.T) {
		p := models.GoalPlan{Monthly: 1_000, StartsOn: now.AddDate(0, -3, 0), StartSaved: 500}
		pr, _ := p.Project(100_000, 2_000, now)
		if !approx(pr.Due, 3_500, 0.01) || !approx(pr.Behind(), 1_500, 0.01) || pr.Ahead() != 0 {
			t.Errorf("due %v, behind %v, ahead %v", pr.Due, pr.Behind(), pr.Ahead())
		}
		pr, _ = p.Project(100_000, 6_000, now)
		if pr.Behind() != 0 || !approx(pr.Ahead(), 1_500, 0.01) {
			t.Errorf("behind %v, ahead %v", pr.Behind(), pr.Ahead())
		}
	})

	t.Run("needs a monthly amount", func(t *testing.T) {
		if _, err := (&models.GoalPlan{}).Project(100, 0, now); !errors.Is(err, models.ErrPlanNoMonthly) {
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
		{ID: "phone", Price: 30_000, Currency: egp, Plan: &models.GoalPlan{Monthly: 5_000}},
		// Saved already, so it takes nothing.
		{ID: "tv", Price: 1_000, Currency: egp, Account: &models.Account{Amount: 1_000}, Plan: &models.GoalPlan{Monthly: 500}},
		{ID: "trip", Price: 10_000, Currency: egp},
	}

	m := models.NewGoalPlanMoney(&car, ws, cats, goals, now)
	if m.Salary != 80_000 || m.Budgets != 20_000 || m.OtherPlans != 5_000 || m.Free() != 55_000 {
		t.Errorf("%+v, free %v", m, m.Free())
	}

	if m := models.NewGoalPlanMoney(&car, &models.WorkSettings{}, cats, goals, now); m.HasSalary() || m.Free() != 0 {
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
