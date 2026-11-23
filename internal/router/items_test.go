package router_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

// TestItems walks the items catalog: lines are linked to items when saved
// (matched by name, picked, or created), merchants follow the transactions,
// items merge, and the page, details, suggestions and the transactions'
// item filter read them.
func TestItems(t *testing.T) {
	repo, ctx, usr := setupTenant(t)
	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.DefaultCurrency = "e2e@example.com", *egp
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	pair, err := jwt.NewPair(usr)
	if err != nil {
		t.Fatal(err)
	}
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	shop := &models.Counterparty{ID: idgen.New(), Name: "Shop", IsMerchant: true}
	mart := &models.Counterparty{ID: idgen.New(), Name: "Mart", IsMerchant: true}
	for _, cp := range []*models.Counterparty{shop, mart} {
		if err := repo.CreateCounterparty(ctx, cp); err != nil {
			t.Fatal(err)
		}
	}

	day := tenantToday().AddDate(0, 0, -10)
	newTx := func(cp *models.Counterparty, daysAgo int, items ...models.TxItem) *models.Transaction {
		t.Helper()
		on := day.AddDate(0, 0, 10-daysAgo)
		tx := &models.Transaction{
			ID: idgen.New(), Account: *bank, Kind: models.ExpenseTxKind, Amount: models.TxItemsTotal(items),
			Counterparty: cp, ScheduledTo: on, FulfilledAt: on, Items: items,
		}
		if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
			t.Fatal(err)
		}
		return tx
	}
	vat := 14.0
	t1 := newTx(shop, 9,
		models.TxItem{Name: "MILK 1L", Quantity: 2, UnitPrice: 10, Total: 20},
		models.TxItem{Name: "Bread", Quantity: 1, UnitPrice: 5, Total: 5},
		models.TxItem{Kind: models.TxItemVAT, Rate: &vat, Total: 3.5},
	)
	t2 := newTx(mart, 5, models.TxItem{Name: "milk  1l.", Quantity: 1, UnitPrice: 12, Total: 12})
	t3 := newTx(nil, 3, models.TxItem{Name: "Eggs", Quantity: 1, Total: 30})

	items, err := repo.GetItems(ctx, *egp, models.ItemsRange{})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]models.ItemSummary{}
	for _, it := range items {
		byName[it.Item.Name] = it
	}
	if len(items) != 3 {
		t.Fatalf("items = %v, want milk, bread, eggs", slices.Collect(func(yield func(string) bool) {
			for n := range byName {
				yield(n)
			}
		}))
	}
	milk := byName["MILK 1L"]
	if milk.Stats.Times != 2 || milk.Merchants != 2 || milk.Stats.Last != 12 || milk.Stats.First != 10 {
		t.Errorf("milk = %+v, merchants %d", milk.Stats, milk.Merchants)
	}
	lines, err := repo.GetTxItems(ctx, t1.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lines {
		if l.Kind.IsCharge() != (l.ItemID == "") {
			t.Errorf("line %q item %q: only items are linked", l.Label(), l.ItemID)
		}
	}

	// A picked item is kept whatever the line's name; an unknown id is
	// dropped for the name's match.
	bread := byName["Bread"].Item.ID
	t4 := newTx(shop, 1,
		models.TxItem{Name: "Baguette", Total: 6, ItemID: bread},
		models.TxItem{Name: "eggs", Total: 31, ItemID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZZ"},
	)
	lines, err = repo.GetTxItems(ctx, t4.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lines[0].ItemID != bread || lines[1].ItemID != byName["Eggs"].Item.ID {
		t.Errorf("t4 lines = %+v", lines)
	}

	// Giving t3 a merchant links its items to it, its lines untouched.
	t3.Items = nil
	upd := *t3
	upd.Counterparty = mart
	if err := repo.UpdateTransaction(ctx, t3, &upd); err != nil {
		t.Fatal(err)
	}
	eggs, err := repo.GetItem(ctx, byName["Eggs"].Item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if eggs.Merchants != 2 {
		t.Errorf("eggs merchants = %d, want mart and shop", eggs.Merchants)
	}

	h := handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)
	srv := httptest.NewServer(router.New(h))
	t.Cleanup(srv.Close)
	do := func(method, path string, form url.Values) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}

	if res, body := do("GET", "/items", nil); res.StatusCode != 200 || !strings.Contains(body, "MILK 1L") || !strings.Contains(body, "Bought 2 times") {
		t.Errorf("items page: %d", res.StatusCode)
	}
	// A range counts only the lines of its days: t2's milk alone.
	from, to := day.AddDate(0, 0, 4).Format(time.DateOnly), day.AddDate(0, 0, 6).Format(time.DateOnly)
	res, body := do("GET", "/items/list?"+url.Values{"range": {"custom"}, "from": {from}, "to": {to}}.Encode(), nil)
	if res.StatusCode != 200 || !strings.Contains(body, "MILK 1L") || strings.Contains(body, "Eggs") || !strings.Contains(body, "Bought once") {
		t.Errorf("items in a range: %d", res.StatusCode)
	}
	wantURL := "/items?" + url.Values{"range": {"custom"}, "from": {from}, "to": {to}}.Encode()
	if got := res.Header.Get("HX-Replace-Url"); got != wantURL {
		t.Errorf("range url = %q", got)
	}
	if _, body := do("GET", "/items/list?range=custom&from=2001-01-01&to=2001-01-02", nil); !strings.Contains(body, "Nothing bought in") {
		t.Error("an empty range doesn't say so")
	}

	if res, body := do("GET", "/items/"+milk.Item.ID+"/details", nil); res.StatusCode != 200 || !strings.Contains(body, "By merchant") || !strings.Contains(body, "Mart") {
		t.Errorf("details: %d", res.StatusCode)
	}
	if res, _ := do("GET", "/items/"+idgen.New()+"/details", nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown item: %d, want 404", res.StatusCode)
	}

	res, body = do("GET", "/items/suggest?"+url.Values{"q": {"MIL"}, "cp": {mart.ID}}.Encode(), nil)
	var sugg []repository.ItemSuggestion
	if err := json.Unmarshal([]byte(body), &sugg); err != nil || res.StatusCode != 200 {
		t.Fatalf("suggest: %d %v", res.StatusCode, err)
	}
	if len(sugg) != 1 || sugg[0].ID != milk.Item.ID || !sugg[0].AtMerchant || sugg[0].LastPrice != 12 || sugg[0].Currency != "EGP" {
		t.Errorf("suggestions = %+v", sugg)
	}

	// Renaming to another item's name is refused; aliases are saved.
	if _, body := do("PUT", "/items/"+bread, url.Values{"name": {"eggs"}}); !strings.Contains(body, "merge them instead") {
		t.Error("renaming onto another item's name went through")
	}
	if res, body := do("PUT", "/items/"+bread, url.Values{"name": {"Bread"}, "aliases": {"Baguette,BREAD"}}); res.StatusCode != 200 || !strings.Contains(body, "Also named") {
		t.Errorf("edit: %d", res.StatusCode)
	}
	b, err := repo.GetItem(ctx, bread)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(b.Item.Aliases, []string{"Baguette"}) {
		t.Errorf("aliases = %v, the name's own key dropped", b.Item.Aliases)
	}
	// A line named like an alias is the item.
	t5 := newTx(mart, 0, models.TxItem{Name: "baguette", Total: 7})
	if lines, _ := repo.GetTxItems(ctx, t5.ID); lines[0].ItemID != bread {
		t.Errorf("alias match: %q, want bread", lines[0].ItemID)
	}

	// The item filter.
	ids, n, err := repo.GetFilteredTransactionIDs(ctx, models.TxFilter{ItemID: milk.Item.ID}, 100)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(ids)
	want := []string{t1.ID, t2.ID}
	slices.Sort(want)
	if n != 2 || !slices.Equal(ids, want) {
		t.Errorf("item filter = %v (%d)", ids, n)
	}
	if res, body := do("GET", "/transactions?item="+milk.Item.ID, nil); res.StatusCode != 200 || !strings.Contains(body, "MILK 1L") {
		t.Errorf("transactions filtered by item: %d", res.StatusCode)
	}

	// Deleting an item on lines is refused; merging moves its lines.
	if res, _ := do("DELETE", "/items/"+bread, nil); res.StatusCode != http.StatusConflict {
		t.Errorf("deleting an item in use: %d, want 409", res.StatusCode)
	}
	if res, body := do("POST", "/items/"+bread+"/merge", url.Values{"into_id": {milk.Item.ID}}); res.StatusCode != 200 || res.Header.Get("HX-Retarget") != "#drawer" || !strings.Contains(body, "Bread") {
		t.Errorf("merge: %d %q", res.StatusCode, res.Header.Get("HX-Retarget"))
	}
	if _, err := repo.GetItem(ctx, bread); err == nil {
		t.Error("the merged item is still there")
	}
	lines, _ = repo.GetTxItems(ctx, t4.ID)
	if lines[0].ItemID != milk.Item.ID {
		t.Errorf("merged line item = %q", lines[0].ItemID)
	}
	m, _ := repo.GetItem(ctx, milk.Item.ID)
	if !slices.Contains(m.Item.Aliases, "Bread") || !slices.Contains(m.Item.Aliases, "Baguette") {
		t.Errorf("merged aliases = %v", m.Item.Aliases)
	}

	// Item categories: quick created from the item form, picked, renamed,
	// a name taken refused, deleted (the item kept, without one).
	if res, body := do("POST", "/items/categories/quick", url.Values{"name": {"Dairy"}}); res.StatusCode != 200 || !strings.Contains(body, "Dairy") {
		t.Fatalf("quick create item category: %d", res.StatusCode)
	}
	dairy, err := repo.FindItemCategory(ctx, "dairy")
	if err != nil || dairy == nil {
		t.Fatalf("item category not created: %v", err)
	}
	if _, body := do("PUT", "/items/"+milk.Item.ID, url.Values{
		"name": {m.Item.Name}, "aliases": {strings.Join(m.Item.Aliases, ",")}, "category_id": {dairy.ID},
	}); !strings.Contains(body, "Dairy") {
		t.Error("the item's category isn't in its details")
	}
	if _, body := do("PUT", "/items/"+milk.Item.ID, url.Values{"name": {m.Item.Name}, "category_id": {"nope"}}); !strings.Contains(body, "Pick a category") {
		t.Error("an unknown category was taken")
	}
	if _, body := do("POST", "/items/categories", url.Values{"name": {"DAIRY"}, "color": {"blue"}}); !strings.Contains(body, "already a category") {
		t.Error("a taken category name went through")
	}
	if res, body := do("PUT", "/items/categories/"+dairy.ID, url.Values{"name": {"Milk & co"}, "color": {"blue"}}); res.StatusCode != 200 || res.Header.Get("HX-Retarget") != "#drawer" || !strings.Contains(body, "Milk &amp; co") {
		t.Errorf("edit item category: %d", res.StatusCode)
	}
	if _, body := do("GET", "/items", nil); !strings.Contains(body, "Milk &amp; co") {
		t.Error("the category isn't a filter on the page")
	}
	if res, _ := do("DELETE", "/items/categories/"+dairy.ID, nil); res.StatusCode != 200 {
		t.Errorf("delete item category: %d", res.StatusCode)
	}
	if m, _ := repo.GetItem(ctx, milk.Item.ID); m.Item.Category != nil {
		t.Errorf("milk category after deleting it = %+v", m.Item.Category)
	}

	// Merging counterparties moves their items.
	if _, err := repo.MergeCounterparties(ctx, mart.ID, shop.ID); err != nil {
		t.Fatal(err)
	}
	if m, _ := repo.GetItem(ctx, milk.Item.ID); m.Merchants != 1 {
		t.Errorf("milk merchants after merging the counterparties = %d", m.Merchants)
	}

	// Resetting the items unlinks the lines.
	if err := repo.Reset(ctx, models.ResetOptions{models.ResetItems: true}); err != nil {
		t.Fatal(err)
	}
	if items, _ := repo.GetItems(ctx, *egp, models.ItemsRange{}); len(items) != 0 {
		t.Errorf("items after reset = %d", len(items))
	}
	if lines, _ := repo.GetTxItems(ctx, t1.ID); lines[0].ItemID != "" || lines[0].Name != "MILK 1L" {
		t.Errorf("line after reset = %+v", lines[0])
	}
}
