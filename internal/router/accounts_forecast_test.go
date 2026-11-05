package router_test

import (
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

// TestAccountsForecastRecurrent checks the accounts summary's forecast counts
// the recurrent transactions' occurrences not scheduled yet.
func TestAccountsForecastRecurrent(t *testing.T) {
	repo, ctx, usr := setupTenant(t)
	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.DefaultCurrency = "e2e@example.com", *egp
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	hd := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)

	// Daily, started 10 days ago: the first fulfilled, the next ones written
	// through next month (the ones until today overdue), the ones after not
	// written yet.
	start := time.Now().AddDate(0, 0, -10).Truncate(24 * time.Hour)
	form := url.Values{
		"kind":         {models.ExpenseTxKind.String()},
		"account_id":   {bank.ID},
		"amount":       {"10"},
		"description":  {"Coffee"},
		"scheduled_to": {start.Format(time.DateOnly)},
		"is_recurrent": {"on"},
		"every":        {"1"},
		"unit":         {strconv.Itoa(int(models.RecurrentUnitDay))},
	}
	if _, err := callHandler(ctx, hd.CreateRecurrentTx)(httptest.NewRecorder(), "POST", "/recurrent-tx", "", form); err != nil {
		t.Fatal(err)
	}

	s, err := repo.GetAccountsSummary(ctx, egp, models.AccountsSummaryWith{BudgetLeft: true})
	if err != nil {
		t.Fatal(err)
	}
	h := s.History
	t0, end := h.TodayIdx(), len(h.Balance)-1
	// The 10 overdue (today's included) tomorrow, then one a day from
	// tomorrow to the end.
	if got, want := h.Balance[end]-h.Balance[t0], -100-10*float64(end-t0); got != want {
		t.Errorf("forecast moved %v, want %v", got, want)
	}
}
