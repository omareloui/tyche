package router_test

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestGoalDetailsURL(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	h := handler.New(repo, valid.New(), sanitizer.New(), nil, nil)

	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := callHandler(ctx, h.CreateGoal)(httptest.NewRecorder(), "POST", "/goals", "", url.Values{
		"name": {"Laptop"}, "amount": {"800"}, "currency_id": {egp.ID},
	}); err != nil {
		t.Fatal(err)
	}
	gls, err := repo.GetGoals(ctx)
	if err != nil || len(gls) != 1 {
		t.Fatalf("goals = %v (%v), want 1", gls, err)
	}
	id := gls[0].ID

	render := func(fn handlerFn, target, id string) string {
		t.Helper()
		c, err := fn(httptest.NewRecorder(), "GET", target, id, nil)
		if err != nil {
			t.Fatal(err)
		}
		var sb strings.Builder
		if err := c.Render(ctx, &sb); err != nil {
			t.Fatal(err)
		}
		return sb.String()
	}

	if body := render(callHandler(ctx, h.Goals), "/goals?goal="+id, ""); !strings.Contains(body, `hx-get="/goals/`+id+`/details" hx-trigger="load"`) {
		t.Error("?goal= doesn't open the details on load")
	}
	if body := render(callHandler(ctx, h.GoalDetails), "/goals/"+id+"/details", id); !strings.Contains(body, `data-url-param="goal" data-url-value="`+id+`"`) {
		t.Error("details don't keep ?goal= in the url")
	}
}
