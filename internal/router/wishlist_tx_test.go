package router_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestWishesBoughtInATransaction(t *testing.T) {
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
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 10000)

	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)))
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

	newWish := func(id, name string, price float64) {
		t.Helper()
		if err := repo.CreateWishlistItem(ctx, &models.WishlistItem{
			ID: id, Name: name, Price: price, Currency: *egp, CoolingDays: 7, CreatedAt: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	const headphones, lamp = "01K8KQZ4N1XQ5WJ5M3Q6G7Y8W1", "01K8KQZ4N1XQ5WJ5M3Q6G7Y8W2"
	newWish(headphones, "Headphones", 1500)
	newWish(lamp, "Desk lamp", 400)

	shop := &models.Counterparty{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8C1", Name: "B.Tech", IsMerchant: true}
	if err := repo.CreateCounterparty(ctx, shop); err != nil {
		t.Fatal(err)
	}
	newTx := func(id, desc string, amount float64, day time.Time, cp *models.Counterparty) *models.Transaction {
		t.Helper()
		tx := &models.Transaction{
			ID: id, Kind: models.ExpenseTxKind, Account: *bank, Amount: amount, Counterparty: cp,
			Description: desc, ScheduledTo: day, FulfilledAt: day, CreatedAt: day,
		}
		if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
			t.Fatal(err)
		}
		return tx
	}
	tx := newTx("01K8KQZ4N1XQ5WJ5M3Q6G7Y8T1", "Electronics store", 2000, time.Now(), shop)
	newTx("01K8KQZ4N1XQ5WJ5M3Q6G7Y8T2", "Coffee", 100, time.Now(), nil)
	newTx("01K8KQZ4N1XQ5WJ5M3Q6G7Y8T3", "Older gadget", 1600, time.Now().AddDate(0, 0, -5), nil)

	t.Run("already bought offers the expenses, the covering ones likely", func(t *testing.T) {
		res, body := do("GET", "/wishlist/"+headphones+"/bought", nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		for _, want := range []string{"Already bought", "Electronics store", "Likely", "B.Tech"} {
			if !strings.Contains(body, want) {
				t.Errorf("picker misses %q", want)
			}
		}
		// Likely first, each group newest first.
		at := func(s string) int { return strings.Index(body, s) }
		if !(at("Electronics store") < at("Older gadget") && at("Older gadget") < at("Coffee")) {
			t.Errorf("order: store %d, older %d, coffee %d", at("Electronics store"), at("Older gadget"), at("Coffee"))
		}
		req, _ := http.NewRequest("GET", srv.URL+"/wishlist/"+headphones+"/bought?q=nothing-like-it", nil)
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Target", "wish-bought-list")
		req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
		r, _ := http.DefaultClient.Do(req)
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if !strings.Contains(string(b), "No expense matches.") || strings.Contains(string(b), "Already bought") {
			t.Errorf("search answer = %s", b)
		}
	})

	t.Run("picking the transaction marks the wish bought in it", func(t *testing.T) {
		res, body := do("PUT", "/wishlist/"+headphones+"/bought", url.Values{"tx_id": {tx.ID}})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		w, _ := repo.GetWishlistItem(ctx, headphones)
		if w.Status != models.WishlistBought || w.TransactionID != tx.ID {
			t.Fatalf("wish = %+v", w)
		}
		if !strings.Contains(body, "See the transaction") {
			t.Error("the bought card doesn't link the transaction")
		}
		if res, _ := do("PUT", "/wishlist/"+headphones+"/bought", url.Values{"tx_id": {tx.ID}}); res.StatusCode != http.StatusConflict {
			t.Errorf("a decided wish again = %d, want 409", res.StatusCode)
		}
	})

	t.Run("the details add another wish, shown as the breakdown", func(t *testing.T) {
		_, body := do("GET", "/transactions/"+tx.ID+"/wishes", nil)
		if !strings.Contains(body, "Desk lamp") || strings.Contains(body, "Headphones") {
			t.Errorf("picker should only offer the waiting wish: %s", body)
		}
		res, body := do("PUT", "/transactions/"+tx.ID+"/wishes", url.Values{"wish_id": {lamp}})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		_, details := do("GET", "/transactions/"+tx.ID+"/details", nil)
		for _, want := range []string{"From your wishlist", "Headphones", "Desk lamp", "Add wishlist item"} {
			if !strings.Contains(details, want) {
				t.Errorf("details miss %q", want)
			}
		}
		_, list := do("GET", "/transactions", nil)
		if !strings.Contains(list, "From your wishlist: Headphones, Desk lamp") {
			t.Error("the row has no wishlist chip")
		}
	})

	t.Run("with lines of its own, the wishes come under them", func(t *testing.T) {
		do("POST", "/transactions/"+tx.ID+"/items", url.Values{"kind": {"0"}, "name": {"Cable"}, "total": {"100"}})
		_, details := do("GET", "/transactions/"+tx.ID+"/details", nil)
		cable, wishes, lampAt := strings.Index(details, "Cable"), strings.Index(details, "From your wishlist"), strings.Index(details, "Desk lamp")
		if cable < 0 || wishes < cable || lampAt < wishes {
			t.Errorf("the wishes aren't under the lines: cable %d, heading %d, lamp %d", cable, wishes, lampAt)
		}
	})

	t.Run("unlinking puts the wish back to waiting", func(t *testing.T) {
		res, _ := do("DELETE", "/transactions/"+tx.ID+"/wishes/"+lamp, nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d", res.StatusCode)
		}
		if w, _ := repo.GetWishlistItem(ctx, lamp); w.Status != models.WishlistWaiting || w.TransactionID != "" {
			t.Errorf("wish = %+v", w)
		}
		if res, _ := do("DELETE", "/transactions/"+tx.ID+"/wishes/"+lamp, nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("unlinking again = %d, want 404", res.StatusCode)
		}
	})

	t.Run("deleting the transaction keeps the wish bought, unlinked", func(t *testing.T) {
		full, _ := repo.GetTransaction(ctx, tx.ID)
		if err := repo.DeleteTransaction(ctx, full); err != nil {
			t.Fatal(err)
		}
		w, _ := repo.GetWishlistItem(ctx, headphones)
		if w.Status != models.WishlistBought || w.TransactionID != "" {
			t.Errorf("wish = %+v", w)
		}
	})
}
