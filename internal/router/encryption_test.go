package router_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

var recoveryCodeRe = regexp.MustCompile(`[A-Z2-7]{4}(-[A-Z2-7]{4}){7}`)

func TestEncryption(t *testing.T) {
	repo, ctx, usr := setupTenant(t)

	usd, err := repo.GetCurrencyByString(ctx, "USD")
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
	refresh := &http.Cookie{Name: "refresh_token", Value: pair.Refresh.Encoded}
	dbPath := "data/" + models.TenantID(usr.OAuthProvider, usr.OAuthID) + ".sqlite"

	newServer := func(repo *repository.Repository) *httptest.Server {
		srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)))
		t.Cleanup(srv.Close)
		return srv
	}
	srv := newServer(repo)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	type reqOpts struct {
		form    url.Values
		htmx    bool
		cookies []*http.Cookie
		headers map[string]string
	}
	do := func(method, path string, o reqOpts) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(o.form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if o.htmx {
			req.Header.Set("HX-Request", "true")
		}
		for k, v := range o.headers {
			req.Header.Set(k, v)
		}
		for _, c := range o.cookies {
			req.AddCookie(c)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}
	cookie := func(res *http.Response, name string) *http.Cookie {
		for _, c := range res.Cookies() {
			if c.Name == name && c.Value != "" {
				return c
			}
		}
		return nil
	}
	readablePlain := func() bool {
		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		_, err = db.Exec("SELECT count(*) FROM sqlite_master")
		return err == nil
	}

	t.Run("asks on the onboarding", func(t *testing.T) {
		res, body := do("GET", "/welcome?step=security", reqOpts{cookies: []*http.Cookie{access}})
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Encrypt your data with a password?") || !strings.Contains(body, "Not now") {
			t.Fatalf("got %d", res.StatusCode)
		}
		if _, body := do("GET", "/accounts", reqOpts{cookies: []*http.Cookie{access}}); strings.Contains(body, `hx-post="/security/lock"`) {
			t.Error("lock button shown for a plain database")
		}
	})

	enable := url.Values{"password": {"correct horse"}, "confirm": {"correct horse"}, "understood": {"1"}}

	t.Run("rejects cross-site and invalid requests", func(t *testing.T) {
		if res, _ := do("POST", "/security/encryption", reqOpts{form: enable, cookies: []*http.Cookie{access}}); res.StatusCode != http.StatusForbidden {
			t.Errorf("without HX-Request: got %d", res.StatusCode)
		}
		for _, form := range []url.Values{
			{"password": {"short"}, "confirm": {"short"}, "understood": {"1"}},
			{"password": {"correct horse"}, "confirm": {"correct house"}, "understood": {"1"}},
			{"password": {"correct horse"}, "confirm": {"correct horse"}},
		} {
			if res, _ := do("POST", "/security/encryption", reqOpts{form: form, htmx: true, cookies: []*http.Cookie{access}}); res.StatusCode != http.StatusUnprocessableEntity {
				t.Errorf("%v: got %d", form, res.StatusCode)
			}
		}
		if !readablePlain() {
			t.Fatal("encrypted after invalid requests")
		}
	})

	var session *http.Cookie
	var code string

	t.Run("encrypts", func(t *testing.T) {
		res, body := do("POST", "/security/encryption", reqOpts{form: enable, htmx: true, cookies: []*http.Cookie{access}})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("got %d: %s", res.StatusCode, body)
		}
		if code = recoveryCodeRe.FindString(body); code == "" {
			t.Fatal("no recovery code shown")
		}
		if session = cookie(res, "vault_session"); session == nil {
			t.Fatal("no vault session cookie")
		}
		if readablePlain() {
			t.Fatal("the database is still readable without the key")
		}
		raw, err := os.ReadFile(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.HasPrefix(raw, []byte("SQLite format 3")) {
			t.Fatal("the database file has a plain header")
		}
	})

	t.Run("reads with the session", func(t *testing.T) {
		res, body := do("GET", "/accounts", reqOpts{cookies: []*http.Cookie{access, session}})
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Cash") {
			t.Fatalf("got %d", res.StatusCode)
		}
		if !strings.Contains(body, `hx-post="/security/lock"`) {
			t.Error("no lock button in the navbar")
		}
		res, body = do("GET", "/security", reqOpts{cookies: []*http.Cookie{access, session}})
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Your data is encrypted") || !strings.Contains(body, "this browser") {
			t.Fatalf("security page: got %d", res.StatusCode)
		}
	})

	t.Run("sends to unlock without the session", func(t *testing.T) {
		res, _ := do("GET", "/accounts?archived=1", reqOpts{cookies: []*http.Cookie{access}})
		if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != middleware.UnlockURL("/accounts?archived=1") {
			t.Fatalf("got %d to %q", res.StatusCode, res.Header.Get("Location"))
		}
		res, _ = do("GET", "/accounts", reqOpts{htmx: true, cookies: []*http.Cookie{access}})
		if res.StatusCode != http.StatusUnauthorized || res.Header.Get("HX-Redirect") == "" {
			t.Fatalf("htmx: got %d to %q", res.StatusCode, res.Header.Get("HX-Redirect"))
		}
		res, _ = do("GET", "/", reqOpts{cookies: []*http.Cookie{access}})
		if res.StatusCode != http.StatusSeeOther {
			t.Fatalf("dashboard: got %d", res.StatusCode)
		}
		forged := &http.Cookie{Name: "vault_session", Value: strings.SplitN(session.Value, ".", 2)[0] + "." + strings.Repeat("A", 43)}
		if res, _ := do("GET", "/accounts", reqOpts{cookies: []*http.Cookie{access, forged}}); res.StatusCode != http.StatusSeeOther {
			t.Fatalf("forged session: got %d", res.StatusCode)
		}
		res, body := do("GET", "/unlock", reqOpts{cookies: []*http.Cookie{access}})
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Unlock your data") {
			t.Fatalf("unlock page: got %d", res.StatusCode)
		}
	})

	outer := t
	t.Run("reopens with the session after a restart", func(t *testing.T) {
		vaults, q := testVaults(outer)
		if err := vaults.Load(ctx); err != nil {
			t.Fatal(err)
		}
		srv = newServer(repository.New(vaults, q))
		res, _ := do("GET", "/accounts", reqOpts{cookies: []*http.Cookie{access, session}})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("got %d", res.StatusCode)
		}
	})

	t.Run("refreshes the tokens with the session", func(t *testing.T) {
		res, _ := do("GET", "/accounts", reqOpts{cookies: []*http.Cookie{refresh, session}})
		if res.StatusCode != http.StatusOK || cookie(res, "access_token") == nil {
			t.Fatalf("got %d", res.StatusCode)
		}
		if res, _ := do("GET", "/accounts", reqOpts{cookies: []*http.Cookie{refresh}}); res.StatusCode != http.StatusSeeOther {
			t.Fatalf("without the session: got %d", res.StatusCode)
		}
	})

	t.Run("unlocks with the password or the recovery code", func(t *testing.T) {
		res, body := do("POST", "/unlock", reqOpts{form: url.Values{"password": {"wrong horse"}, "next": {"/accounts"}}, cookies: []*http.Cookie{access}})
		if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "Wrong password") || cookie(res, "vault_session") != nil {
			t.Fatalf("wrong password: got %d", res.StatusCode)
		}

		res, _ = do("POST", "/unlock", reqOpts{form: url.Values{"password": {"correct horse"}, "next": {"/accounts"}}, cookies: []*http.Cookie{access}})
		if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/accounts" || cookie(res, "vault_session") == nil {
			t.Fatalf("password: got %d to %q", res.StatusCode, res.Header.Get("Location"))
		}

		// Only the refresh token: signs in too.
		res, _ = do("POST", "/unlock", reqOpts{form: url.Values{"password": {strings.ToLower(code)}, "next": {"//evil.example"}}, cookies: []*http.Cookie{refresh}})
		if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/" {
			t.Fatalf("recovery code: got %d to %q", res.StatusCode, res.Header.Get("Location"))
		}
		if cookie(res, "vault_session") == nil || cookie(res, "access_token") == nil {
			t.Fatal("the recovery code didn't sign in")
		}
	})

	t.Run("locks from any page", func(t *testing.T) {
		res, _ := do("POST", "/unlock", reqOpts{form: url.Values{"password": {"correct horse"}}, cookies: []*http.Cookie{access}})
		s := cookie(res, "vault_session")
		if s == nil {
			t.Fatal("no vault session cookie")
		}
		res, _ = do("POST", "/security/lock", reqOpts{htmx: true, cookies: []*http.Cookie{access, s}, headers: map[string]string{"HX-Current-URL": srv.URL + "/accounts?archived=1"}})
		if want := middleware.UnlockURL("/accounts?archived=1"); res.Header.Get("HX-Redirect") != want {
			t.Fatalf("got %d to %q, want %q", res.StatusCode, res.Header.Get("HX-Redirect"), want)
		}
		if res, _ = do("GET", "/accounts", reqOpts{cookies: []*http.Cookie{access, s}}); res.StatusCode != http.StatusSeeOther {
			t.Fatalf("the session still reads after locking: got %d", res.StatusCode)
		}
	})

	t.Run("a device has its own key", func(t *testing.T) {
		key, err := repo.Vaults().Unlock(ctx, models.TenantID(usr.OAuthProvider, usr.OAuthID), "correct horse")
		if err != nil {
			t.Fatal(err)
		}
		keyCtx := withKey(ctx, key)
		device := &models.Device{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZE", Name: "phone", TokenID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZF"}
		if err := repo.RegisterDevice(keyCtx, device); err != nil {
			t.Fatal(err)
		}
		secret, err := repo.Vaults().NewDeviceSlot(ctx, models.TenantID(usr.OAuthProvider, usr.OAuthID), device.ID, key)
		if err != nil {
			t.Fatal(err)
		}

		post := func(secret string) int {
			token, _ := jwt.NewDeviceToken(usr, device.ID, device.TokenID, secret)
			b, _ := json.Marshal(map[string]any{"messages": []map[string]any{{"sender": "HSBC", "body": "Your OTP is 1234", "received_at": time.Now()}}})
			req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/sms", bytes.NewReader(b))
			req.Header.Set("Authorization", "Bearer "+token)
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			return res.StatusCode
		}
		if got := post(secret); got != http.StatusOK {
			t.Fatalf("with its key: got %d", got)
		}
		if got := post(""); got != http.StatusUnauthorized {
			t.Fatalf("without its key: got %d", got)
		}

		if res, _ := do("PUT", "/devices/"+device.ID+"/revoke", reqOpts{htmx: true, cookies: []*http.Cookie{access, session}}); res.StatusCode != http.StatusOK {
			t.Fatalf("revoking: got %d", res.StatusCode)
		}
		if _, err := repo.Vaults().OpenDevice(ctx, models.TenantID(usr.OAuthProvider, usr.OAuthID), device.ID, secret); err == nil {
			t.Fatal("the revoked device's key still works")
		}
	})

	t.Run("changes the password with the recovery code", func(t *testing.T) {
		form := url.Values{"current": {code}, "password": {"battery staple"}, "confirm": {"battery staple"}}
		if res, body := do("POST", "/security/password", reqOpts{form: form, htmx: true, cookies: []*http.Cookie{access, session}}); res.StatusCode != http.StatusOK || !strings.Contains(body, "Password changed") {
			t.Fatalf("got %d: %s", res.StatusCode, body)
		}
		if res, _ := do("POST", "/unlock", reqOpts{form: url.Values{"password": {"correct horse"}}, cookies: []*http.Cookie{access}}); res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("old password: got %d", res.StatusCode)
		}
		if res, _ := do("POST", "/unlock", reqOpts{form: url.Values{"password": {"battery staple"}}, cookies: []*http.Cookie{access}}); res.StatusCode != http.StatusSeeOther {
			t.Fatalf("new password: got %d", res.StatusCode)
		}
	})

	t.Run("turns it off", func(t *testing.T) {
		if res, _ := do("POST", "/security/encryption/disable", reqOpts{form: url.Values{"password": {"correct horse"}}, htmx: true, cookies: []*http.Cookie{access, session}}); res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("wrong password: got %d", res.StatusCode)
		}
		if res, body := do("POST", "/security/encryption/disable", reqOpts{form: url.Values{"password": {"battery staple"}}, htmx: true, cookies: []*http.Cookie{access, session}}); res.StatusCode != http.StatusOK {
			t.Fatalf("got %d: %s", res.StatusCode, body)
		}
		if !readablePlain() {
			t.Fatal("the database isn't plain again")
		}
		if res, body := do("GET", "/accounts", reqOpts{cookies: []*http.Cookie{access}}); res.StatusCode != http.StatusOK || !strings.Contains(body, "Cash") {
			t.Fatalf("reading it: got %d", res.StatusCode)
		}
	})
}

func withKey(ctx context.Context, key []byte) context.Context {
	return context.WithValue(ctx, middleware.VaultKeyCtxKey{}, key)
}
