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
		{Status: models.WishlistAbandoned, Price: 999, Installments: 3, InstallmentAmount: 400, Currency: egp},
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
	acc := models.Account{Currency: egp}
	return models.Goal{
		ID: id, Name: id, Price: price, Currency: egp,
		Transactions: []models.Transaction{{Amount: saved, Account: acc, FulfilledAt: time.Now()}},
	}
}

func TestCheckAffordability(t *testing.T) {
	balances := []models.Balance{{Amount: 10000, Currency: egp}}
	ws := &models.WorkSettings{MonthlySalary: 52000, Currency: egp, HoursPerWeek: 40}
	goals := []models.Goal{goalWithSaved("car", 100000, 20000), goalWithSaved("trip", 5000, 5000)}
	paces := models.GoalPaces{"car": 2000}

	t.Run("cash", func(t *testing.T) {
		a := models.CheckAffordability(models.Purchase{Price: 3000, Currency: egp}, balances, goals, paces, ws, &egp)
		if a.BalanceNow != 10000 || a.BalanceAfter != 7000 || a.BalanceAfterAll != 7000 || a.Markup != 0 {
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

	t.Run("installments", func(t *testing.T) {
		p := models.Purchase{Price: 12000, Currency: egp, Installments: 12, InstallmentAmount: 1100}
		a := models.CheckAffordability(p, balances, nil, nil, ws, &egp)
		if a.Upfront != 1100 || a.TotalCost != 13200 || a.Markup != 1200 || a.BalanceAfter != 8900 || a.BalanceAfterAll != -3200 {
			t.Errorf("affordability = %+v", a)
		}
		if a.Verdict != models.VerdictCareful {
			t.Errorf("Verdict = %v, want careful", a.Verdict)
		}
	})

	t.Run("too expensive", func(t *testing.T) {
		a := models.CheckAffordability(models.Purchase{Price: 20000, Currency: egp}, balances, nil, nil, ws, &egp)
		if a.Verdict != models.VerdictNo {
			t.Errorf("Verdict = %v, want no", a.Verdict)
		}
	})

	t.Run("cheap, no salary, no pace", func(t *testing.T) {
		a := models.CheckAffordability(models.Purchase{Price: 100, Currency: egp}, balances, goals, nil, &models.WorkSettings{HoursPerWeek: 40}, &egp)
		if a.Verdict != models.VerdictYes || a.WorkHours != 0 || a.SalaryShare != 0 || a.Goals[0].DelayMonths != -1 {
			t.Errorf("affordability = %+v", a)
		}
	})
}
