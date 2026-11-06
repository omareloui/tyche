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

	if body := render(callHandler(ctx, h.AffordDrawer), "GET", "/decide/afford", nil); !strings.Contains(body, `name="on"`) || !strings.Contains(body, `name="term_by"`) {
		t.Error("the drawer has no day or term toggle")
	}

	body := render(callHandler(ctx, h.Afford), "POST", "/decide/afford", url.Values{
		"amount": {"100"}, "mode": {"cash"}, "on": {"2030-01-01"},
	})
	if !strings.Contains(body, "on Jan 1, 2030") {
		t.Errorf("the verdict isn't on the day:\n%s", body)
	}
	// Without a salary, it all comes out of the balance.
	if !strings.Contains(body, "Set your monthly salary") {
		t.Error("no hint to set the salary")
	}

	body = render(callHandler(ctx, h.Afford), "POST", "/decide/afford", url.Values{
		"amount": {"1200"}, "mode": {"installments"}, "strategy": {"known"}, "term_by": {"amount"}, "installment_amount": {"100"},
	})
	if !strings.Contains(body, "12 payments") {
		t.Errorf("per month didn't work the months out:\n%s", body)
	}
}
