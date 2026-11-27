package router_test

import (
	"bytes"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/badge"
	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestReturnWarrantyWindows(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	hd := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)
	today := tenantToday()

	shop := &models.Counterparty{ID: idgen.New(), Name: "Electro", IsMerchant: true, ReturnDays: 14}
	if err := repo.CreateCounterparty(ctx, shop); err != nil {
		t.Fatal(err)
	}
	newTx := func(t *testing.T, kind models.TxKind, items []models.TxItem) *models.Transaction {
		t.Helper()
		tx := &models.Transaction{
			ID: idgen.New(), Account: *bank, Kind: kind, Amount: 1000, Counterparty: &models.Counterparty{ID: shop.ID},
			ScheduledTo: today, FulfilledAt: time.Now(), Items: items,
		}
		if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
			t.Fatal(err)
		}
		return tx
	}
	put := func(t *testing.T, fn handler.HandlerMethod, id, itemID string, form url.Values) (string, error) {
		t.Helper()
		r := httptest.NewRequest("PUT", "/x", strings.NewReader(form.Encode())).WithContext(ctx)
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.SetPathValue("id", id)
		r.SetPathValue("itemID", itemID)
		c, err := fn(httptest.NewRecorder(), r)
		if err != nil {
			return "", err
		}
		var b bytes.Buffer
		if err := c.Render(ctx, &b); err != nil {
			t.Fatal(err)
		}
		return b.String(), nil
	}

	t.Run("presets, a day, returned", func(t *testing.T) {
		tx := newTx(t, models.ExpenseTxKind, nil)
		if _, err := put(t, hd.SetTransactionWindows, tx.ID, "", url.Values{"return": {"14d"}, "warranty": {"24m"}}); err != nil {
			t.Fatal(err)
		}
		got, _ := repo.GetTransaction(ctx, tx.ID)
		if got.ReturnUntil == nil || !got.ReturnUntil.Equal(today.AddDate(0, 0, 14)) {
			t.Errorf("return until = %v", got.ReturnUntil)
		}
		if got.WarrantyUntil == nil || !got.WarrantyUntil.Equal(models.AddMonthsClamped(today, 24)) {
			t.Errorf("warranty = %v", got.WarrantyUntil)
		}

		// Only the sent ones change.
		body, err := put(t, hd.SetTransactionWindows, tx.ID, "", url.Values{"returned": {"1"}})
		if err != nil {
			t.Fatal(err)
		}
		got, _ = repo.GetTransaction(ctx, tx.ID)
		if got.ReturnedAt == nil || got.ReturnUntil == nil || got.WarrantyUntil == nil {
			t.Errorf("after returned: %v %v %v", got.ReturnedAt, got.ReturnUntil, got.WarrantyUntil)
		}
		if !strings.Contains(body, ">Returned ") || !strings.Contains(body, "Add refund") {
			t.Errorf("answer: %s", body)
		}

		// Before the purchase is refused, nothing saved.
		body, _ = put(t, hd.SetTransactionWindows, tx.ID, "", url.Values{"return": {today.AddDate(0, 0, -1).Format(time.DateOnly)}})
		if !strings.Contains(body, "can&#39;t end before") {
			t.Errorf("not refused: %s", body)
		}
		put(t, hd.SetTransactionWindows, tx.ID, "", url.Values{"return": {""}, "returned": {"0"}})
		got, _ = repo.GetTransaction(ctx, tx.ID)
		if got.ReturnUntil != nil || got.ReturnedAt != nil {
			t.Errorf("not cleared: %v %v", got.ReturnUntil, got.ReturnedAt)
		}
	})

	t.Run("only expenses", func(t *testing.T) {
		tx := newTx(t, models.IncomeTxKind, nil)
		if _, err := put(t, hd.SetTransactionWindows, tx.ID, "", url.Values{"return": {"14d"}}); err == nil {
			t.Error("an income took a return window")
		}
	})

	t.Run("a line's warranty, filter and badge", func(t *testing.T) {
		tx := newTx(t, models.ExpenseTxKind, []models.TxItem{
			{Name: "TV", Quantity: 1, UnitPrice: 900, Total: 900},
			{Kind: models.TxItemVAT, Total: 100},
		})
		full, _ := repo.GetTransaction(ctx, tx.ID)
		var tv, vat string
		for _, it := range full.Items {
			if it.Kind.IsCharge() {
				vat = it.ID
			} else {
				tv = it.ID
			}
		}
		soon := today.AddDate(0, 0, 10).Format(time.DateOnly)
		if _, err := put(t, hd.SetTxItemWarranty, tx.ID, tv, url.Values{"warranty": {soon}}); err != nil {
			t.Fatal(err)
		}
		if _, err := put(t, hd.SetTxItemWarranty, tx.ID, vat, url.Values{"warranty": {soon}}); err == nil {
			t.Error("a charge took a warranty")
		}
		full, _ = repo.GetTransaction(ctx, tx.ID)
		if d, own := full.Items[0].EffectiveWarranty(full); !own || d.Format(time.DateOnly) != soon {
			t.Errorf("line warranty = %v %t", d, own)
		}

		// Matched through its line alone.
		ids, _, err := repo.GetFilteredTransactionIDs(ctx, models.TxFilter{Window: models.TxWindowWarrantyActive}, -1)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(ids, tx.ID) {
			t.Errorf("warranty_active misses it: %v", ids)
		}
		// The list's chip knows it without the lines.
		page, err := repo.GetTransactionsPage(ctx, models.TxFilter{Window: models.TxWindowWarrantyActive}, time.Time{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, lt := range page.History {
			if lt.ID == tx.ID && (lt.LatestWarranty() == nil || lt.LatestWarranty().Format(time.DateOnly) != soon) {
				t.Errorf("list's latest warranty = %v", lt.LatestWarranty())
			}
		}

		counts, err := repo.GetBadgeCounts(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var tags []string
		for _, it := range counts.Items {
			if it.Source == badge.SourceWarranty {
				tags = append(tags, it.Tag)
			}
		}
		if !slices.Contains(tags, "warranty:"+tv) || counts.Warranty < 1 {
			t.Errorf("badge: %d %v", counts.Warranty, tags)
		}
	})

	t.Run("edit a line, keeping its warranty", func(t *testing.T) {
		tx := newTx(t, models.ExpenseTxKind, []models.TxItem{
			{Name: "TV", Quantity: 1, UnitPrice: 900, Total: 900},
			{Kind: models.TxItemVAT, Total: 100},
		})
		full, _ := repo.GetTransaction(ctx, tx.ID)
		tv := full.Items[0].ID
		vat := full.Items[1].ID
		body, err := put(t, hd.UpdateTxItem, tx.ID, tv, url.Values{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(body, "Name the item.") {
			t.Errorf("a nameless item saved: %s", body)
		}
		// No warranty: the add button, no chip.
		if !strings.Contains(body, `title="Add a warranty"`) {
			t.Errorf("no add warranty button: %s", body)
		}

		soon := today.AddDate(0, 0, 10).Format(time.DateOnly)
		if _, err := put(t, hd.SetTxItemWarranty, tx.ID, tv, url.Values{"warranty": {soon}}); err != nil {
			t.Fatal(err)
		}
		body, err = put(t, hd.UpdateTxItem, tx.ID, tv, url.Values{
			"kind": {"0"}, "name": {"TV 55"}, "quantity": {"2"}, "unit_price": {"450"}, "total": {""},
		})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(body, `title="Add a warranty"`) {
			t.Errorf("add warranty shown with one")
		}
		full, _ = repo.GetTransaction(ctx, tx.ID)
		var got *models.TxItem
		for i := range full.Items {
			if full.Items[i].ID == tv {
				got = &full.Items[i]
			}
		}
		if got == nil || got.Name != "TV 55" || got.Quantity != 2 || got.Total != 900 || got.ItemID == "" {
			t.Fatalf("edited = %+v", got)
		}
		if got.WarrantyUntil == nil || got.WarrantyUntil.Format(time.DateOnly) != soon {
			t.Errorf("warranty lost: %v", got.WarrantyUntil)
		}

		// A charge turned into a service with a rate.
		if _, err := put(t, hd.UpdateTxItem, tx.ID, vat, url.Values{"kind": {strconv.Itoa(int(models.TxItemService))}, "rate": {"10"}}); err != nil {
			t.Fatal(err)
		}
		full, _ = repo.GetTransaction(ctx, tx.ID)
		for _, it := range full.Items {
			if it.ID == vat && (it.Kind != models.TxItemService || it.Total != 90) {
				t.Errorf("charge = %+v", it)
			}
		}
		if _, err := put(t, hd.UpdateTxItem, tx.ID, "nope", url.Values{"kind": {"0"}, "name": {"X"}}); err == nil {
			t.Error("an unknown line edited")
		}
	})

	t.Run("return open filter and badge", func(t *testing.T) {
		tx := newTx(t, models.ExpenseTxKind, nil)
		put(t, hd.SetTransactionWindows, tx.ID, "", url.Values{"return": {today.AddDate(0, 0, 2).Format(time.DateOnly)}})
		ids, _, _ := repo.GetFilteredTransactionIDs(ctx, models.TxFilter{Window: models.TxWindowReturnOpen}, -1)
		if !slices.Contains(ids, tx.ID) {
			t.Errorf("return_open misses it")
		}
		counts, _ := repo.GetBadgeCounts(ctx)
		found := false
		for _, it := range counts.Items {
			found = found || it.Tag == "return:"+tx.ID
		}
		if !found {
			t.Errorf("no return reminder: %+v", counts.Items)
		}
		put(t, hd.SetTransactionWindows, tx.ID, "", url.Values{"returned": {"1"}})
		ids, _, _ = repo.GetFilteredTransactionIDs(ctx, models.TxFilter{Window: models.TxWindowReturnOpen}, -1)
		if slices.Contains(ids, tx.ID) {
			t.Errorf("a returned one is still open")
		}
	})

	t.Run("merchant default on create, kept on edit", func(t *testing.T) {
		form := url.Values{
			"kind": {models.ExpenseTxKind.String()}, "account_id": {bank.ID}, "amount": {"50"},
			"counterparty_id": {shop.ID}, "scheduled_to": {today.Format(time.DateOnly)},
			"item_name": {"Mouse"}, "item_kind": {"0"}, "item_quantity": {"1"}, "item_unit_price": {"50"},
			"item_total": {"50"}, "item_id": {""}, "item_item_id": {""}, "item_rate": {""}, "item_included": {""},
			"item_warranty_until": {today.AddDate(1, 0, 0).Format(time.DateOnly)},
		}
		w := httptest.NewRecorder()
		if _, err := callHandler(ctx, hd.CreateTransaction)(w, "POST", "/transactions", "", form); err != nil {
			t.Fatal(err)
		}
		page, err := repo.GetTransactionsPage(ctx, models.TxFilter{Q: "", CounterpartyID: shop.ID}, time.Time{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		var created *models.Transaction
		for i := range page.History {
			if page.History[i].Amount == 50 {
				created = &page.History[i]
			}
		}
		if created == nil {
			t.Fatal("not created")
		}
		got, _ := repo.GetTransaction(ctx, created.ID)
		if got.ReturnUntil == nil || !got.ReturnUntil.Equal(today.AddDate(0, 0, 14)) {
			t.Errorf("merchant default = %v", got.ReturnUntil)
		}
		if len(got.Items) != 1 || got.Items[0].WarrantyUntil == nil {
			t.Fatalf("line warranty not saved: %+v", got.Items)
		}

		// An edit form without the windows' fields leaves them alone.
		edit := url.Values{
			"kind": {models.ExpenseTxKind.String()}, "account_id": {bank.ID}, "amount": {"55"},
			"counterparty_id": {shop.ID}, "scheduled_to": {today.Format(time.DateOnly)},
			"item_name": {"Mouse"}, "item_kind": {"0"}, "item_quantity": {"1"}, "item_unit_price": {"55"},
			"item_total": {"55"}, "item_id": {got.Items[0].ID}, "item_item_id": {got.Items[0].ItemID}, "item_rate": {""},
			"item_included": {""}, "item_warranty_until": {got.Items[0].WarrantyUntil.Format(time.DateOnly)},
		}
		if _, err := callHandler(ctx, hd.EditTransaction)(httptest.NewRecorder(), "POST", "/x", got.ID, edit); err != nil {
			t.Fatal(err)
		}
		after, _ := repo.GetTransaction(ctx, got.ID)
		if after.ReturnUntil == nil || len(after.Items) != 1 || after.Items[0].WarrantyUntil == nil {
			t.Errorf("edit dropped them: %v %+v", after.ReturnUntil, after.Items)
		}

		// With them, cleared.
		edit.Set("windows", "1")
		edit.Set("return_until", "")
		callHandler(ctx, hd.EditTransaction)(httptest.NewRecorder(), "POST", "/x", got.ID, edit)
		if after, _ := repo.GetTransaction(ctx, got.ID); after.ReturnUntil != nil {
			t.Errorf("not cleared: %v", after.ReturnUntil)
		}
	})
}
