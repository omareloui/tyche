package router_test

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

// The goal plan's free money is the month saving starts in's: the other
// goals' plans done by then take nothing.
func TestGoalPlanMoneyOfStartMonth(t *testing.T) {
	repo, ctx, usr := setupTenant(t)
	h := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)

	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.DefaultCurrency = "e2e@example.com", *egp
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetWorkSettings(ctx, &models.WorkSettings{MonthlySalary: 10000, Currency: *egp, HoursPerWeek: 40}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Phone", "Car"} {
		if _, err := callHandler(ctx, h.CreateGoal)(httptest.NewRecorder(), "POST", "/goals", "", url.Values{
			"name": {name}, "amount": {"6000"}, "currency_id": {egp.ID},
		}); err != nil {
			t.Fatal(err)
		}
	}
	gls, err := repo.GetGoals(ctx)
	if err != nil || len(gls) != 2 {
		t.Fatalf("goals = %v (%v)", gls, err)
	}
	phone, car := gls[0], gls[1]
	if phone.Name != "Phone" {
		phone, car = car, phone
	}
	// 2,000 a month: done in 3 months.
	if _, err := callHandler(ctx, h.SetGoalPlan)(httptest.NewRecorder(), "PUT", "/goals/x/plan", phone.ID, url.Values{
		"plan_by": {"monthly"}, "monthly": {"2000"}, "mode": {"save"},
	}); err != nil {
		t.Fatal(err)
	}

	preview := func(startsOn string) string {
		t.Helper()
		c, err := callHandler(ctx, h.GoalPlanPreview)(httptest.NewRecorder(), "POST", "/goals/x/plan/preview", car.ID, url.Values{
			"plan_by": {"monthly"}, "monthly": {"1000"}, "mode": {"save"}, "starts_on": {startsOn},
		})
		if err != nil {
			t.Fatal(err)
		}
		var sb strings.Builder
		if err := c.Render(ctx, &sb); err != nil {
			t.Fatal(err)
		}
		return sb.String()
	}
	if body := preview(""); !strings.Contains(body, "Other goals&#39; plans") {
		t.Errorf("this month has no other plans:\n%.3000s", body)
	}
	if body := preview(""); !strings.Contains(body, `id="plan-`+car.ID+`-money" hx-swap-oob="true"`) {
		t.Errorf("what's free isn't out of band:\n%.3000s", body)
	}
	// 6,000 at 1,000 a month: the deposits listed under the cards.
	if body := preview(""); !strings.Contains(body, "Saving schedule · 6 deposits") {
		t.Errorf("no saving schedule:\n%.3000s", body)
	}
	if gls, err = repo.GetGoals(ctx); err != nil {
		t.Fatal(err)
	}
	ms := models.PlanMilestones(&car, &models.WorkSettings{MonthlySalary: 10000, Currency: *egp, HoursPerWeek: 40}, nil, gls, models.MonthStart(1), time.Now())
	if len(ms) == 0 {
		t.Fatal("no milestones")
	}
	done := ms[len(ms)-1].On
	if body := preview(done.Format(time.DateOnly)); strings.Contains(body, "Other goals&#39; plans") {
		t.Errorf("after the phone's plan (%v) it still takes money:\n%s", done, body)
	}

	// The same in "Plan it", for something not a goal yet.
	plan := func(startsOn string) string {
		t.Helper()
		c, err := callHandler(ctx, h.Afford)(httptest.NewRecorder(), "POST", "/decide/afford", "", url.Values{
			"name": {"Bike"}, "amount": {"6000"}, "currency_id": {egp.ID},
			"plan_by": {"monthly"}, "monthly": {"1000"}, "mode": {"save"}, "starts_on": {startsOn},
		})
		if err != nil {
			t.Fatal(err)
		}
		var sb strings.Builder
		if err := c.Render(ctx, &sb); err != nil {
			t.Fatal(err)
		}
		return sb.String()
	}
	if body := plan(""); !strings.Contains(body, "Other goals&#39; plans") {
		t.Errorf("plan it: this month has no other plans:\n%.2000s", body)
	}
	if body := plan(done.Format(time.DateOnly)); strings.Contains(body, "Other goals&#39; plans") {
		t.Errorf("plan it: after the plans (%v) they still take money:\n%.2000s", done, body)
	}
}
