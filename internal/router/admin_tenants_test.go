package router_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestAdminTenants(t *testing.T) {
	repo, ctx, admin := setupTenant(t)
	adminID := models.TenantID(admin.OAuthProvider, admin.OAuthID)
	t.Setenv("ADMIN_TENANT_IDS", adminID)

	// The admin's database, a plain user's and an encrypted one's vault left
	// without its database.
	if _, err := repo.GetBalances(ctx); err != nil {
		t.Fatal(err)
	}
	plain := &models.User{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZB", OAuthID: "plain", OAuthProvider: models.Google}
	plainID := models.TenantID(plain.OAuthProvider, plain.OAuthID)
	plainCtx := context.WithValue(context.Background(), middleware.AccessClaimsCtxKey{}, &jwt.AccessClaims{
		ID: plain.ID, OAuthID: plain.OAuthID, OAuthProvider: plain.OAuthProvider,
	})
	if _, err := repo.GetBalances(plainCtx); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("data/"+plainID+".sqlite.bak", []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	lockedID := models.TenantID(models.Google, "locked")
	if _, _, err := repo.Vaults().Create(ctx, lockedID, "correct horse"); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)))
	defer srv.Close()

	tokens := func(usr *models.User) string {
		t.Helper()
		pair, err := jwt.NewPair(usr)
		if err != nil {
			t.Fatal(err)
		}
		return pair.Access.Encoded
	}
	adminToken, plainToken := tokens(admin), tokens(plain)

	do := func(method, path, token string, form url.Values, htmx bool) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if htmx {
			req.Header.Set("HX-Request", "true")
		}
		req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}
	exists := func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}

	t.Run("only admins", func(t *testing.T) {
		if res, _ := do("GET", "/admin/tenants", plainToken, nil, false); res.StatusCode != http.StatusNotFound {
			t.Errorf("list: got %d", res.StatusCode)
		}
		res, _ := do("POST", "/admin/tenants/"+adminID+"/delete", plainToken, url.Values{"confirm": {adminID}}, true)
		if res.StatusCode != http.StatusNotFound || !exists("data/"+adminID+".sqlite") {
			t.Errorf("delete: got %d", res.StatusCode)
		}
	})

	t.Run("lists every database, plain or encrypted", func(t *testing.T) {
		res, body := do("GET", "/admin/tenants", adminToken, nil, false)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("got %d", res.StatusCode)
		}
		for _, want := range []string{adminID, plainID, lockedID, "Encrypted", "Plain", "Database file missing", "Import backup", "3 accounts, 1 encrypted"} {
			if !strings.Contains(body, want) {
				t.Errorf("the list is missing %q", want)
			}
		}
	})

	t.Run("shows the account ID in the user menu", func(t *testing.T) {
		_, body := do("GET", "/accounts", plainToken, nil, false)
		if !strings.Contains(body, plainID) {
			t.Error("no account ID")
		}
	})

	t.Run("refuses", func(t *testing.T) {
		for name, c := range map[string]struct {
			id, confirm string
			htmx        bool
			code        int
		}{
			"without htmx":   {plainID, plainID, false, http.StatusNotFound},
			"wrong confirm":  {plainID, "nope", true, http.StatusUnprocessableEntity},
			"own account":    {adminID, adminID, true, http.StatusUnprocessableEntity},
			"unknown":        {"GL_nobody", "GL_nobody", true, http.StatusNotFound},
			"invalid":        {"GL_..", "GL_..", true, http.StatusNotFound},
			"traversal path": {"..%2Fshared", "../shared", true, http.StatusNotFound},
		} {
			res, _ := do("POST", "/admin/tenants/"+c.id+"/delete", adminToken, url.Values{"confirm": {c.confirm}}, c.htmx)
			if res.StatusCode != c.code {
				t.Errorf("%s: got %d, want %d", name, res.StatusCode, c.code)
			}
		}
		if !exists("data/"+plainID+".sqlite") || !exists("data/shared.sqlite") {
			t.Error("a database was deleted")
		}
	})

	t.Run("deletes an account", func(t *testing.T) {
		res, body := do("POST", "/admin/tenants/"+plainID+"/delete", adminToken, url.Values{"confirm": {plainID}}, true)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Deleted") {
			t.Fatalf("got %d %s", res.StatusCode, body)
		}
		if exists("data/"+plainID+".sqlite") || exists("data/"+plainID+".sqlite.bak") {
			t.Error("the database files are still there")
		}

		// Its old cookies sign it out, without creating the database again.
		res, _ = do("GET", "/accounts", plainToken, nil, false)
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("old cookie: got %d", res.StatusCode)
		}
		if _, err := repo.GetBalances(plainCtx); !errors.Is(err, repository.ErrTenantDeleted) {
			t.Errorf("repo: got %v", err)
		}
		if exists("data/" + plainID + ".sqlite") {
			t.Error("the database was created again")
		}

		// Until it signs in again.
		if err := repo.Undelete(ctx, plainID); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.GetBalances(plainCtx); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("deletes an encrypted account's vault", func(t *testing.T) {
		res, _ := do("POST", "/admin/tenants/"+lockedID+"/delete", adminToken, url.Values{"confirm": {lockedID}}, true)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("got %d", res.StatusCode)
		}
		if repo.Vaults().IsEncrypted(lockedID) {
			t.Error("the vault is still there")
		}
	})

	t.Run("deleted stays deleted after a restart", func(t *testing.T) {
		restarted := testRepo(t)
		if err := restarted.LoadDeleted(ctx); err != nil {
			t.Fatal(err)
		}
		if !restarted.IsDeleted(lockedID) || restarted.IsDeleted(plainID) {
			t.Error("wrong deleted tenants after a restart")
		}
	})
}
