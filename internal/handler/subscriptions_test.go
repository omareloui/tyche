package handler

import (
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/web/views"
)

// monthlyCharges creates n fulfilled expenses at cp, a month apart, the last
// one 10 days ago.
func (e *pickEnv) monthlyCharges(t *testing.T, cp *models.Counterparty, cat *models.Category, n int, amount float64) {
	t.Helper()
	last := time.Now().UTC().AddDate(0, 0, -10)
	last = time.Date(last.Year(), last.Month(), min(last.Day(), 28), 0, 0, 0, 0, time.UTC)
	for i := range n {
		day := last.AddDate(0, -i, 0)
		tx := &models.Transaction{
			ID: idgen.New(), Account: e.acc, Amount: amount, Kind: models.ExpenseTxKind,
			Category: cat, Counterparty: cp, ScheduledTo: day, FulfilledAt: day,
		}
		if err := e.repo.CreateTransaction(e.ctx, tx, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
}

func (e *pickEnv) suggestions(t *testing.T) []views.SubscriptionSuggestion {
	t.Helper()
	d, err := e.h.subscriptionSuggestions(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return d.Items
}

func TestSubscriptionSuggestions(t *testing.T) {
	e := newPickEnv(t)
	cat := e.category(t, models.ExpenseTxKind, "")
	cp := &models.Counterparty{ID: idgen.New(), Name: "Streamer", IsMerchant: true}
	if err := e.repo.CreateCounterparty(e.ctx, cp); err != nil {
		t.Fatal(err)
	}
	e.monthlyCharges(t, cp, cat, 5, 199)

	got := e.suggestions(t)
	if len(got) != 1 {
		t.Fatalf("got %d suggestions, want 1", len(got))
	}
	s := got[0]
	if s.CounterpartyID != cp.ID || s.Unit != models.RecurrentUnitMonth || s.Every != 1 || s.Amount != 199 || s.CategoryID != cat.ID {
		t.Errorf("suggestion = %+v", s.SubscriptionCandidate)
	}
	path := map[string]string{"cp": cp.ID, "cur": s.CurrencyID}

	// The form is prefilled.
	r := httptest.NewRequest("GET", "/", nil).WithContext(e.ctx)
	for k, v := range path {
		r.SetPathValue(k, v)
	}
	c, err := e.h.SubscriptionForm(httptest.NewRecorder(), r)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	var b strings.Builder
	if err := c.Render(e.ctx, &b); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	for _, want := range []string{"from=" + views.SubscriptionFormFrom, cp.ID, cat.ID, s.NextFrom(time.Now().AddDate(0, 0, 1)).Format(time.DateOnly)} {
		if !strings.Contains(html, want) {
			t.Errorf("form lacks %q", want)
		}
	}

	// Saving it makes the recurrent transaction, the suggestion gone.
	form := url.Values{
		"account_id": {e.acc.ID}, "amount": {"199"}, "kind": {models.ExpenseTxKind.String()},
		"counterparty_id": {cp.ID}, "category_id": {cat.ID},
		"scheduled_to": {s.NextFrom(time.Now().AddDate(0, 0, 1)).Format(time.DateOnly)},
		"is_recurrent": {"on"}, "every": {"1"}, "unit": {strconv.Itoa(int(models.RecurrentUnitMonth))},
	}
	w, r := e.put("/recurrent-tx?from="+views.SubscriptionFormFrom, form, nil)
	r.Method = "POST"
	if _, err := e.h.CreateRecurrentTx(w, r); err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := w.Header().Get("HX-Retarget"); got != "#"+views.SubscriptionSuggestionsID {
		t.Errorf("HX-Retarget = %q, want the suggestions", got)
	}
	rtxs, err := e.repo.GetRecurrentTxs(e.ctx)
	if err != nil || len(rtxs) != 1 {
		t.Fatalf("recurrent txs: %v (%d)", err, len(rtxs))
	}
	if got := e.suggestions(t); len(got) != 0 {
		t.Errorf("still suggested: %+v", got)
	}
}

func TestDismissSubscription(t *testing.T) {
	e := newPickEnv(t)
	cp := &models.Counterparty{ID: idgen.New(), Name: "Gym", IsMerchant: true}
	if err := e.repo.CreateCounterparty(e.ctx, cp); err != nil {
		t.Fatal(err)
	}
	e.monthlyCharges(t, cp, nil, 4, 500)
	got := e.suggestions(t)
	if len(got) != 1 {
		t.Fatalf("got %d suggestions, want 1", len(got))
	}

	w, r := e.put("/", nil, map[string]string{"cp": cp.ID, "cur": got[0].CurrencyID})
	r.Method = "POST"
	if _, err := e.h.DismissSubscription(w, r); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if got := e.suggestions(t); len(got) != 0 {
		t.Errorf("dismissed one still suggested: %+v", got)
	}

	// A dismissed one is a 404.
	if _, err := e.h.DismissSubscription(w, r); statusOf(err) != 404 {
		t.Errorf("dismissing again: err = %v, want 404", err)
	}

	// The amount changing much brings it back.
	if err := e.repo.DismissSubscription(e.ctx, cp.ID, got[0].CurrencyID, 300); err != nil {
		t.Fatal(err)
	}
	if got := e.suggestions(t); len(got) != 1 {
		t.Errorf("got %d suggestions after the amount changed, want 1", len(got))
	}
}
