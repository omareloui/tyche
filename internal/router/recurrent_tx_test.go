package router_test

import (
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestCreateRecurrentTx(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	hd := handler.New(repo, valid.New(), sanitizer.New(), nil, nil)

	// Started in the past: the first one is fulfilled, the next ones counted
	// from its day, not today.
	start := time.Now().AddDate(0, 0, -10).Truncate(24 * time.Hour)
	form := url.Values{
		"kind":         {models.ExpenseTxKind.String()},
		"account_id":   {bank.ID},
		"amount":       {"100"},
		"description":  {"Rent"},
		"scheduled_to": {start.Format(time.DateOnly)},
		"is_recurrent": {"on"},
		"every":        {"1"},
		"unit":         {strconv.Itoa(int(models.RecurrentUnitMonth))},
	}

	w := httptest.NewRecorder()
	if _, err := callHandler(ctx, hd.CreateRecurrentTx)(w, "POST", "/recurrent-tx", "", form); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(w.Header().Get("HX-Trigger"), "drawer-saved") {
		t.Fatalf("HX-Trigger = %q, want drawer-saved", w.Header().Get("HX-Trigger"))
	}
	if !strings.Contains(w.Body.String(), `id="rtx-list"`) {
		t.Error("answer misses the list out of band")
	}

	rts, err := repo.GetRecurrentTxs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rts) != 1 {
		t.Fatalf("recurrent txs = %d, want 1", len(rts))
	}
	rt, err := repo.GetRecurrentTx(ctx, rts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if rt.FulfilledCount != 1 || len(rt.Upcoming) != 5 {
		t.Errorf("fulfilled %d, upcoming %d, want 1 and 5", rt.FulfilledCount, len(rt.Upcoming))
	}
	if start.Day() <= 28 {
		for i, d := range rt.Upcoming {
			if d.Day() != start.Day() {
				t.Errorf("upcoming[%d] = %v, want on day %d", i, d, start.Day())
			}
		}
	}

	if bank, err = repo.GetAccount(ctx, bank.ID); err != nil {
		t.Fatal(err)
	}
	if bank.Amount != 900 {
		t.Errorf("bank balance = %v, want 900", bank.Amount)
	}

	// Its details list them.
	c, err := callHandler(ctx, hd.RecurrentTxDetails)(httptest.NewRecorder(), "GET", "/recurrent-tx/"+rt.ID+"/details", rt.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := c.Render(ctx, &b); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Rent details", `data-url-param="rtx"`, "Upcoming", "History"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("details miss %q", want)
		}
	}

	// No amount: the form answers with its errors, nothing created.
	form.Del("amount")
	w = httptest.NewRecorder()
	if _, err := callHandler(ctx, hd.CreateRecurrentTx)(w, "POST", "/recurrent-tx", "", form); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.Header().Get("HX-Trigger"), "drawer-saved") {
		t.Error("invalid form closed the drawer")
	}
	if rts, _ = repo.GetRecurrentTxs(ctx); len(rts) != 1 {
		t.Errorf("recurrent txs = %d, want still 1", len(rts))
	}
}
