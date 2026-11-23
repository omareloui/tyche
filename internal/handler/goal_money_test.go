package handler

import (
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/web/views"
)

var toAmountValue = regexp.MustCompile(`name="to_amount"\s+value="([^"]*)"`)

// A withdraw across currencies works what the account receives out again
// as the amount is typed, even once it has a value.
func TestGoalWithdrawReceivedAmount(t *testing.T) {
	e := newPickEnv(t)
	now := time.Now().UTC()
	def := e.acc.Currency

	curs, err := e.repo.GetCurrencies(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	var other *models.Currency
	for _, code := range []string{"EGP", "USD"} {
		for i := range curs {
			if strings.EqualFold(curs[i].String, code) && !strings.EqualFold(code, def.String) {
				other = &curs[i]
				break
			}
		}
		if other != nil {
			break
		}
	}
	if other == nil {
		t.Fatalf("no EGP/USD currency seeded besides %q: %d currencies", def.String, len(curs))
	}
	t.Cleanup(fxrate.SetRates(map[string]map[string]float64{
		strings.ToLower(def.String): {strings.ToLower(other.String): 0.02},
	}))

	dest := &models.Account{ID: idgen.New(), Name: "Abroad", Currency: *other, Kind: e.acc.Kind, SmsRefs: []string{}}
	if err := e.repo.CreateAccount(e.ctx, dest); err != nil {
		t.Fatal(err)
	}

	gl := &models.Goal{ID: idgen.New(), Name: "Trip", Price: 1000, Currency: def, CreatedAt: now}
	if err := e.repo.CreateGoal(e.ctx, gl); err != nil {
		t.Fatal(err)
	}
	if gl, err = e.repo.GetGoal(e.ctx, gl.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.DepositToGoal(e.ctx, gl, &models.Transaction{
		ID: idgen.New(), Kind: models.TransferTxKind, Account: e.acc, Amount: 500,
		ScheduledTo: now, FulfilledAt: now, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	received := func(trigger, amount, toAmount string) string {
		t.Helper()
		form := url.Values{
			"dir": {string(views.GoalWithdraw)}, "amount": {amount}, "account_id": {dest.ID},
			"holding_id": {gl.Account.ID}, "to_amount": {toAmount},
		}
		r := httptest.NewRequest("POST", "/goals/"+gl.ID+"/money-inputs", strings.NewReader(form.Encode())).WithContext(e.ctx)
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("HX-Request", "true")
		r.Header.Set("HX-Trigger-Name", trigger)
		r.SetPathValue("id", gl.ID)
		c, err := e.h.GoalMoneyInputs(httptest.NewRecorder(), r)
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		if err := c.Render(e.ctx, &b); err != nil {
			t.Fatal(err)
		}
		m := toAmountValue.FindStringSubmatch(b.String())
		if m == nil {
			t.Fatalf("no to_amount input in:\n%s", b.String())
		}
		return m[1]
	}

	// Typing the amount replaces the received amount worked out before.
	if got := received("amount", "100", "1"); got != "2" {
		t.Errorf("typed amount: receives %q, want 2", got)
	}
	// Another change keeps one typed by hand.
	if got := received("", "100", "1.5"); got != "1.5" {
		t.Errorf("no trigger: receives %q, want the typed 1.5", got)
	}
}
