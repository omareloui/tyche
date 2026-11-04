package router_test

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestOnboarding(t *testing.T) {
	repo, ctx, usr := setupTenant(t)

	usd, err := repo.GetCurrencyByString(ctx, "USD")
	if err != nil {
		t.Fatal(err)
	}
	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	aed, err := repo.GetCurrencyByString(ctx, "AED")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.FirstName, usr.LastName, usr.DefaultCurrency = "e2e@example.com", "End", "ToEnd", *usd
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	pair, err := jwt.NewPair(usr)
	if err != nil {
		t.Fatal(err)
	}
	access := &http.Cookie{Name: "access_token", Value: pair.Access.Encoded}

	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)))
	t.Cleanup(srv.Close)

	do := func(method, path string, form url.Values, htmx bool) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if htmx {
			req.Header.Set("HX-Request", "true")
			req.Header.Set("HX-Target", "welcome")
		}
		req.AddCookie(access)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}
	accountsByName := func() map[string]models.Account {
		t.Helper()
		accs, err := repo.GetAccounts(ctx)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]models.Account{}
		for _, a := range accs {
			m[a.Name] = a
		}
		return m
	}
	kindID := func(enum models.AccKind) string {
		t.Helper()
		kinds, err := repo.GetAccountKinds(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range kinds {
			if k.Enum == enum {
				return k.ID
			}
		}
		t.Fatalf("no kind %v", enum)
		return ""
	}

	t.Run("suggests the region from the browser", func(t *testing.T) {
		res, body := do("GET", "/welcome", nil, false)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, `hx-trigger="load"`) {
			t.Fatalf("got %d, no suggestion loader", res.StatusCode)
		}
		res, body = do("GET", "/welcome?step=region&tz=Asia/Dubai&langs=en-US", nil, true)
		if res.StatusCode != http.StatusOK || strings.Contains(body, "<html") {
			t.Fatalf("got %d, want the step alone", res.StatusCode)
		}
		if !strings.Contains(body, `value="Asia/Dubai"`) || !strings.Contains(body, aed.ID) {
			t.Error("didn't suggest Dubai and AED")
		}
	})

	t.Run("rejects cross-site requests", func(t *testing.T) {
		if res, _ := do("POST", "/welcome/region", url.Values{"currency_id": {egp.ID}, "timezone": {"Africa/Cairo"}}, false); res.StatusCode != http.StatusForbidden {
			t.Errorf("got %d", res.StatusCode)
		}
	})

	t.Run("rejects an unknown time zone", func(t *testing.T) {
		res, body := do("POST", "/welcome/region", url.Values{"currency_id": {egp.ID}, "timezone": {"Mars/Olympus"}}, true)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Unknown time zone") {
			t.Fatalf("got %d", res.StatusCode)
		}
	})

	t.Run("sets the region", func(t *testing.T) {
		res, body := do("POST", "/welcome/region", url.Values{"currency_id": {egp.ID}, "timezone": {"Asia/Dubai"}}, true)
		if res.StatusCode != http.StatusOK || res.Header.Get("HX-Push-Url") != "/welcome?step=accounts" {
			t.Fatalf("got %d to %q: %s", res.StatusCode, res.Header.Get("HX-Push-Url"), body)
		}
		var refreshed bool
		for _, c := range res.Cookies() {
			refreshed = refreshed || (c.Name == "access_token" && c.Value != "")
		}
		if !refreshed {
			t.Error("the access token wasn't refreshed")
		}
		cur, err := repo.GetDefaultCurrency(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if cur.ID != egp.ID {
			t.Errorf("default currency %s, want EGP", cur.String)
		}
		loc, err := repo.GetTimezone(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if loc.String() != "Asia/Dubai" {
			t.Errorf("time zone %s", loc)
		}
		if c := accountsByName()["Cash"].Currency.ID; c != egp.ID {
			t.Errorf("the seeded Cash account didn't move to EGP")
		}
	})

	t.Run("adds accounts with their balances", func(t *testing.T) {
		cash := accountsByName()["Cash"]
		res, body := do("POST", "/welcome/accounts", url.Values{
			"amount-" + cash.ID: {"1,500.00"},
			"new_kind_id":       {kindID(models.AccKindCreditCard)},
			"new_name":          {"Visa"},
			"new_currency_id":   {egp.ID},
			"new_amount":        {"2,000"},
		}, true)
		if res.StatusCode != http.StatusOK || res.Header.Get("HX-Push-Url") != "/welcome?step=accounts" {
			t.Fatalf("got %d to %q: %s", res.StatusCode, res.Header.Get("HX-Push-Url"), body)
		}
		accs := accountsByName()
		if accs["Cash"].Amount != 1500 {
			t.Errorf("Cash balance %v", accs["Cash"].Amount)
		}
		if visa, ok := accs["Visa"]; !ok || visa.Amount != -2000 || visa.Kind.Enum != models.AccKindCreditCard {
			t.Errorf("Visa: %+v", visa)
		}
		if !strings.Contains(body, "Visa") {
			t.Error("the added account isn't listed")
		}

		res, body = do("POST", "/welcome/accounts", url.Values{
			"new_kind_id": {kindID(models.AccKindBankAcc)}, "new_name": {"Visa"}, "new_currency_id": {egp.ID},
		}, true)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "already have an account") {
			t.Fatalf("duplicate: got %d", res.StatusCode)
		}
		res, body = do("POST", "/welcome/accounts", url.Values{"new_kind_id": {kindID(models.AccKindDebt)}, "new_name": {"Ali"}, "new_currency_id": {egp.ID}}, true)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Pick a kind.") {
			t.Fatalf("debt: got %d", res.StatusCode)
		}
	})

	t.Run("saves the balances and moves on", func(t *testing.T) {
		accs := accountsByName()
		res, body := do("POST", "/welcome/balances", url.Values{
			"amount-" + accs["Cash"].ID: {"100"},
			"amount-" + accs["Visa"].ID: {"2,000.00"},
		}, true)
		if res.StatusCode != http.StatusOK || res.Header.Get("HX-Push-Url") != "/welcome?step=security" || !strings.Contains(body, "Encrypt your data") {
			t.Fatalf("got %d to %q", res.StatusCode, res.Header.Get("HX-Push-Url"))
		}
		accs = accountsByName()
		if accs["Cash"].Amount != 100 || accs["Visa"].Amount != -2000 {
			t.Errorf("balances: Cash %v, Visa %v", accs["Cash"].Amount, accs["Visa"].Amount)
		}
		txs, err := repo.GetTransactions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(txs) != 0 {
			t.Errorf("got %d transactions, want the balances as is", len(txs))
		}
	})

	t.Run("finishes", func(t *testing.T) {
		res, _ := do("POST", "/welcome/finish", url.Values{"next": {"/sms"}}, true)
		if res.StatusCode != http.StatusOK || res.Header.Get("HX-Redirect") != "/sms" {
			t.Fatalf("got %d to %q", res.StatusCode, res.Header.Get("HX-Redirect"))
		}
		got, err := repo.GetUserByOAuthID(ctx, usr.OAuthID)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Onboarded {
			t.Error("not marked as onboarded")
		}
		res, _ = do("POST", "/welcome/finish", url.Values{"next": {"//evil.example"}}, true)
		if res.Header.Get("HX-Redirect") != "/" {
			t.Errorf("open redirect to %q", res.Header.Get("HX-Redirect"))
		}
	})

	// Last: the requests after it need the vault session.
	t.Run("encrypts from the security step", func(t *testing.T) {
		res, body := do("POST", "/security/encryption", url.Values{
			"password": {"correct horse"}, "confirm": {"correct horse"}, "understood": {"1"}, "welcome": {"1"},
		}, true)
		if res.StatusCode != http.StatusOK || res.Header.Get("HX-Retarget") != "#welcome" || res.Header.Get("HX-Push-Url") != "/welcome?step=security" {
			t.Fatalf("got %d to %q", res.StatusCode, res.Header.Get("HX-Retarget"))
		}
		if recoveryCodeRe.FindString(body) == "" || !strings.Contains(body, "I saved it") {
			t.Error("no recovery code with the continue button")
		}
	})
}

// TestOnboardingMigration checks the users there before the onboarding are
// done with it.
func TestOnboardingMigration(t *testing.T) {
	setupTenant(t)

	db, err := sql.Open("sqlite3", "data/migration.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpTo(db, "sql/schema", 20240909183050); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO currencies (id, string, symbol, kind) VALUES ('c', 'USD', '$', 'Fiat')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user (id, oauth_id, oauth_provider, email, first_name, last_name, default_currency_id)
		VALUES ('u', 'o', 1, 'e@example.com', 'Fi', 'La', 'c')`); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(db, "sql/schema"); err != nil {
		t.Fatal(err)
	}
	var onboarded sql.NullTime
	if err := db.QueryRow("SELECT onboarded_at FROM user").Scan(&onboarded); err != nil {
		t.Fatal(err)
	}
	if !onboarded.Valid {
		t.Error("existing user not marked as onboarded")
	}
	if _, err := db.Exec("SELECT encryption_prompted_at FROM user"); err == nil {
		t.Error("encryption_prompted_at still there")
	}
}
