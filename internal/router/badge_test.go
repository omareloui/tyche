package router_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/badge"
	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestBadgeCounts(t *testing.T) {
	repo, ctx, usr := setupTenant(t)
	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.FirstName, usr.LastName, usr.DefaultCurrency = "badge@example.com", "Badge", "User", *egp
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	kinds, err := repo.GetAccountKinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var bankKind models.AccountKind
	for _, k := range kinds {
		if k.Enum == models.AccKindBankAcc {
			bankKind = k
		}
	}
	bank := &models.Account{ID: idgen.New(), Name: "Bank", Amount: 1000, Currency: *egp, Kind: bankKind}
	if err := repo.CreateAccount(ctx, bank); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for _, tx := range []models.Transaction{
		// Overdue: scheduled before today, not fulfilled.
		{ScheduledTo: today.AddDate(0, 0, -3)},
		{ScheduledTo: today.Add(-time.Minute)},
		// Not overdue: fulfilled, due today or later.
		{ScheduledTo: today.AddDate(0, 0, -2), FulfilledAt: today.AddDate(0, 0, -2)},
		{ScheduledTo: today.Add(time.Hour)},
		{ScheduledTo: today.AddDate(0, 0, 5)},
		// To review.
		{ScheduledTo: today.AddDate(0, 0, -1), FulfilledAt: today.AddDate(0, 0, -1), NeedsReview: true},
	} {
		tx.ID, tx.Account, tx.Kind, tx.Amount = idgen.New(), *bank, models.ExpenseTxKind, 10
		if err := repo.CreateTransaction(ctx, &tx, nil, nil); err != nil {
			t.Fatal(err)
		}
	}

	got, err := repo.GetBadgeCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.SMS != 0 || got.Review != 1 || got.Overdue != 2 {
		t.Fatalf("counts: got %+v", got)
	}
	if len(got.Items) != 3 || got.Items[0].Source != badge.SourceReview ||
		!strings.HasPrefix(got.Items[0].Title, "To review · ") || got.Items[0].URL != "/transactions?tx="+strings.TrimPrefix(got.Items[0].Tag, "tx:") ||
		got.Items[1].Source != badge.SourceOverdue || !strings.Contains(got.Items[1].Body, " · due ") {
		t.Fatalf("items: got %+v", got.Items)
	}

	pair, err := jwt.NewPair(usr)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)))
	t.Cleanup(srv.Close)
	do := func(method, path, body string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
		req.Header.Set("HX-Request", "true")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}

	t.Run("GET /badge", func(t *testing.T) {
		res, body := do(http.MethodGet, "/badge", "")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("got %d: %s", res.StatusCode, body)
		}
		var c badge.Counts
		if err := json.Unmarshal([]byte(body), &c); err != nil {
			t.Fatal(err)
		}
		for i := range c.Items {
			c.Items[i].Source = got.Items[i].Source // not sent
		}
		if !reflect.DeepEqual(c, got) {
			t.Errorf("got %+v, want %+v", c, got)
		}
	})

	t.Run("no VAPID keys, no pushes", func(t *testing.T) {
		for _, path := range []string{"/push/key"} {
			if res, _ := do(http.MethodGet, path, ""); res.StatusCode != http.StatusNotFound {
				t.Errorf("GET %s: got %d", path, res.StatusCode)
			}
		}
		if res, _ := do(http.MethodPut, "/push/subscription", `{}`); res.StatusCode != http.StatusNotFound {
			t.Errorf("PUT: got %d", res.StatusCode)
		}
	})
}
