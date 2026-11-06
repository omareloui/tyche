package router_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestAffordOnADay(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	h := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)

	render := func(fn handlerFn, method, target string, form url.Values) string {
		t.Helper()
		c, err := fn(httptest.NewRecorder(), method, target, "", form)
		if err != nil {
			t.Fatal(err)
		}
		var sb strings.Builder
		if err := c.Render(ctx, &sb); err != nil {
			t.Fatal(err)
		}
		return sb.String()
	}

	if body := render(callHandler(ctx, h.AffordDrawer), "GET", "/decide/afford", nil); !strings.Contains(body, `name="starts_on"`) || !strings.Contains(body, `name="plan_by"`) || !strings.Contains(body, `name="term_by"`) {
		t.Error("the drawer has no start day, save by or term toggles")
	}

	body := render(callHandler(ctx, h.Afford), "POST", "/decide/afford", url.Values{
		"amount": {"300"}, "mode": {"save"}, "plan_by": {"months"}, "months": {"3"}, "starts_on": {"2030-01-15"},
	})
	// Saving from January 15 over 3 months: bought in March.
	if !strings.Contains(body, "Mar 2030") || !strings.Contains(body, "on Mar 1, 2030") {
		t.Errorf("not saved from the start day:\n%s", body)
	}
	// Without a salary, it all comes out of the balance.
	if !strings.Contains(body, "Set your monthly salary") {
		t.Error("no hint to set the salary")
	}

	body = render(callHandler(ctx, h.Afford), "POST", "/decide/afford", url.Values{
		"amount": {"1200"}, "mode": {"installments"}, "plan_by": {"monthly"}, "monthly": {"100"},
		"strategy": {"known"}, "term_by": {"amount"}, "installment_amount": {"100"},
	})
	if !strings.Contains(body, "12 payments") {
		t.Errorf("per month didn't work the months out:\n%s", body)
	}

	// Without a name, a 422 the button shows.
	_, err := callHandler(ctx, h.SaveAffordGoal)(httptest.NewRecorder(), "POST", "/decide/goal", "", url.Values{
		"amount": {"3000"}, "mode": {"save"}, "plan_by": {"monthly"}, "monthly": {"500"},
	})
	var re *resperr.RespError
	if !errors.As(err, &re) || re.Code != http.StatusUnprocessableEntity {
		t.Errorf("without a name: %v", err)
	}

	// Saved as a goal with its plan.
	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	if c, err := callHandler(ctx, h.SaveAffordGoal)(w, "POST", "/decide/goal", "", url.Values{
		"name": {"Bike"}, "amount": {"3000"}, "currency_id": {egp.ID}, "mode": {"save"}, "plan_by": {"months"}, "months": {"6"}, "starts_on": {"2030-03-15"},
	}); err != nil {
		t.Fatal(err)
	} else if c != nil {
		var sb strings.Builder
		_ = c.Render(ctx, &sb)
		t.Errorf("not saved: %s", sb.String())
	}
	if !strings.HasPrefix(w.Header().Get("HX-Location"), "/goals?goal=") {
		t.Errorf("HX-Location = %q", w.Header().Get("HX-Location"))
	}
	gls, err := repo.GetGoals(ctx)
	if err != nil || len(gls) != 1 || gls[0].Name != "Bike" || gls[0].Plan == nil || gls[0].Plan.Months != 6 || gls[0].Plan.Monthly != 500 || !gls[0].Plan.StartsOn.Equal(time.Date(2030, 3, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("goals = %+v (%v)", gls, err)
	}
}
