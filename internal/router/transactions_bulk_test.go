package router_test

import (
	"bytes"
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestTransactionsBulk(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	h := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)

	food := &models.Category{ID: idgen.New(), Name: "Food", Color: "red", Kinds: []models.TxKind{models.ExpenseTxKind}}
	if err := repo.CreateCategory(ctx, food); err != nil {
		t.Fatal(err)
	}
	shop := &models.Counterparty{ID: idgen.New(), Name: "Shop", IsMerchant: true}
	if err := repo.CreateCounterparty(ctx, shop); err != nil {
		t.Fatal(err)
	}

	past := time.Now().UTC().AddDate(0, 0, -3).Truncate(time.Hour)
	create := func(kind models.TxKind, amount float64, cat *models.Category, fulfilled bool) *models.Transaction {
		t.Helper()
		tx := &models.Transaction{ID: idgen.New(), Account: *bank, Kind: kind, Amount: amount, Category: cat, ScheduledTo: past}
		if fulfilled {
			tx.FulfilledAt = past
		} else {
			tx.ScheduledTo = time.Now().UTC().AddDate(0, 0, 5)
		}
		if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
			t.Fatal(err)
		}
		return tx
	}
	e1 := create(models.ExpenseTxKind, 10, nil, true)
	e2 := create(models.ExpenseTxKind, 20, food, true)
	i1 := create(models.IncomeTxKind, 50, nil, true)
	up := create(models.ExpenseTxKind, 5, nil, false)

	none := models.TxFilter{CategoryID: models.TxFilterNone}
	ids, rows, err := repo.GetFilteredTransactionIDs(ctx, none, -1)
	if err != nil {
		t.Fatal(err)
	}
	if rows != 3 || !sameIDs(ids, e1.ID, i1.ID, up.ID) {
		t.Errorf("uncategorized = %v (%d rows), want e1, i1, up", ids, rows)
	}
	s, err := repo.GetTransactionsSummary(ctx, none, &bank.Currency)
	if err != nil {
		t.Fatal(err)
	}
	if s.Count != rows {
		t.Errorf("summary count = %d, want the list's %d", s.Count, rows)
	}
	if ids, _, _ := repo.GetFilteredTransactionIDs(ctx, models.TxFilter{CounterpartyID: models.TxFilterNone}, -1); len(ids) != 4 {
		t.Errorf("no counterparty = %d, want 4", len(ids))
	}

	render := func(c templ.Component, err error) string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		if err := c.Render(ctx, &b); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	bulk := callHandler(ctx, h.BulkEditTransactions)

	// The food category fits the expense, not the income.
	w := httptest.NewRecorder()
	render(bulk(w, "POST", "/transactions/bulk-edit", "", url.Values{
		"id": {e1.ID, i1.ID}, "category_id": {food.ID}, "counterparty_id": {shop.ID},
	}))
	if got := w.Header().Get("HX-Trigger"); !strings.Contains(got, `"tx-bulk-done":{"skipped":1,"updated":2}`) {
		t.Errorf("HX-Trigger = %q, want 2 updated, 1 skipped", got)
	}
	if tx := mustTx(t, repo.GetTransaction, ctx, e1.ID); tx.Category == nil || tx.Category.ID != food.ID || tx.Counterparty == nil || tx.Counterparty.ID != shop.ID {
		t.Errorf("e1 = %+v, %+v, want food at shop", tx.Category, tx.Counterparty)
	}
	if tx := mustTx(t, repo.GetTransaction, ctx, i1.ID); tx.Category != nil || tx.Counterparty == nil {
		t.Errorf("i1 = %+v, %+v, want no category, at shop", tx.Category, tx.Counterparty)
	}

	// Every one matching the filter, refused when the count moved.
	all := url.Values{"all": {"1"}, models.TxFilterCategory: {models.TxFilterNone}, "counterparty_id": {models.TxFilterNone}}
	all.Set("expect", "5")
	if body := render(bulk(httptest.NewRecorder(), "POST", "/transactions/bulk-edit", "", all)); !strings.Contains(body, "changed meanwhile") {
		t.Errorf("wrong expect not refused: %s", body)
	}
	all.Set("expect", "2")
	render(bulk(httptest.NewRecorder(), "POST", "/transactions/bulk-edit", "", all))
	if tx := mustTx(t, repo.GetTransaction, ctx, i1.ID); tx.Counterparty != nil {
		t.Errorf("i1 counterparty = %+v, want removed", tx.Counterparty)
	}

	// A fulfilled one can't move ahead of today, a scheduled one can.
	tomorrow := time.Now().UTC().AddDate(0, 0, 2).Format(time.DateOnly)
	w = httptest.NewRecorder()
	render(bulk(w, "POST", "/transactions/bulk-edit", "", url.Values{"id": {e2.ID, up.ID}, "scheduled_to": {tomorrow}}))
	if tx := mustTx(t, repo.GetTransaction, ctx, e2.ID); !tx.ScheduledTo.Equal(past) {
		t.Errorf("fulfilled moved to %v", tx.ScheduledTo)
	}
	if tx := mustTx(t, repo.GetTransaction, ctx, up.ID); tx.ScheduledTo.Format(time.DateOnly) != tomorrow {
		t.Errorf("scheduled at %v, want %s", tx.ScheduledTo, tomorrow)
	}

	if body := render(bulk(httptest.NewRecorder(), "POST", "/transactions/bulk-edit", "", url.Values{"id": {e1.ID}})); !strings.Contains(body, "Pick something") {
		t.Errorf("empty edit not refused: %s", body)
	}

	// Deleting the filter's: the food ones (e1, e2), the balance undone.
	del := callHandler(ctx, h.DeleteTransactions)
	render(del(httptest.NewRecorder(), "POST", "/transactions/delete", "", url.Values{
		"all": {"1"}, models.TxFilterCategory: {food.ID}, "expect": {"2"},
	}))
	for _, id := range []string{e1.ID, e2.ID} {
		if _, err := repo.GetTransaction(ctx, id); err == nil {
			t.Errorf("%s not deleted", id)
		}
	}
	acc, err := repo.GetAccount(ctx, bank.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := 1000.0 + 50; acc.Amount != want {
		t.Errorf("balance = %v, want %v", acc.Amount, want)
	}

	// Opening the form drops (and tells) the ones that can't be edited.
	adj := create(models.AdjustmentTxKind, 5, nil, true)
	form := callHandler(ctx, h.BulkEditTransactionsForm)
	w = httptest.NewRecorder()
	body := render(form(w, "POST", "/transactions/bulk-edit/form", "", url.Values{"id": {i1.ID, adj.ID}}))
	if got := w.Header().Get("HX-Trigger"); !strings.Contains(got, "tx-sel-drop") || !strings.Contains(got, adj.ID) || strings.Contains(got, i1.ID) {
		t.Errorf("HX-Trigger = %q, want the adjustment dropped", got)
	}
	if !strings.Contains(body, "1 balance correction") || !strings.Contains(body, "Edit 1 transaction") {
		t.Errorf("form doesn't tell the adjustment left out: %s", body)
	}
	w = httptest.NewRecorder()
	render(form(w, "POST", "/transactions/bulk-edit/form", "", url.Values{"id": {adj.ID}}))
	if w.Header().Get("HX-Reswap") != "none" || !strings.Contains(w.Header().Get("HX-Trigger"), "can't be edited") {
		t.Errorf("only locked ones: headers = %v", w.Header())
	}
	// Left out of the edit even when sent (all of the filter's).
	render(bulk(httptest.NewRecorder(), "POST", "/transactions/bulk-edit", "", url.Values{"id": {adj.ID}, "category_id": {food.ID}}))
	if tx := mustTx(t, repo.GetTransaction, ctx, adj.ID); tx.Category != nil {
		t.Errorf("adjustment edited: %+v", tx.Category)
	}
}

func mustTx(t *testing.T, get func(context.Context, string) (*models.Transaction, error), ctx context.Context, id string) *models.Transaction {
	t.Helper()
	tx, err := get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func sameIDs(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]int{}
	for _, id := range got {
		seen[id]++
	}
	for _, id := range want {
		seen[id]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}
