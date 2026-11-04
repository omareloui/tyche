package handler

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/shareddb"
	"github.com/omareloui/tyche/internal/valid"
	"github.com/omareloui/tyche/internal/vault"
)

// TestSetupKeepsTheVault signs up again a tenant whose encrypted database file
// was deleted while its vault was kept: the new database is encrypted with the
// vault's key (unlocked by the browser's session), instead of failing as
// locked.
func TestSetupKeepsTheVault(t *testing.T) {
	schema, err := filepath.Abs("../../sql")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	t.Chdir(tmp)
	if err := os.Symlink(schema, filepath.Join(tmp, "sql")); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"data", "logs"} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("JWT_SECRET", "test-secret")

	db, q, err := shareddb.Open("data/shared.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	vaults := vault.New(db, q, vault.KDFParams{Time: 1, Memory: 1024, Threads: 1})

	gUsr := func() *models.User {
		return &models.User{
			ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZA", OAuthID: "again", OAuthProvider: models.Google,
			Email: "again@example.com", FirstName: "Sign", LastName: "Again",
		}
	}
	ctx := context.WithValue(context.Background(), middleware.AccessClaimsCtxKey{}, &jwt.AccessClaims{
		OAuthID: "again", OAuthProvider: models.Google,
	})

	// The first life: signed up, then encrypted.
	repo := repository.New(vaults, q)
	h := New(repo, valid.New(), sanitizer.New(), nil, nil, nil)
	if err := h.initUser(ctx, gUsr()); err != nil {
		t.Fatal(err)
	}
	key, _, err := repo.EnableEncryption(ctx, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	dbPath := "data/" + models.TenantID(models.Google, "again") + ".sqlite"
	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}

	// Signing in again, from a fresh server.
	repo = repository.New(vaults, q)
	h = New(repo, valid.New(), sanitizer.New(), nil, nil, nil)
	id, err := pendingSignups.add(gUsr(), key)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/oauth/setup", nil)
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: signupPendingCookieName, Value: id})
	rec := httptest.NewRecorder()
	if _, err := h.OAuthSetup(rec, req); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if got := rec.Header().Get("HX-Redirect"); got != "/welcome" {
		t.Fatalf("redirected to %q", got)
	}

	plain, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	if _, err := plain.Exec("SELECT count(*) FROM sqlite_master"); err == nil {
		t.Error("the new database isn't encrypted")
	}
	usr, err := repo.GetUserByOAuthID(context.WithValue(ctx, middleware.VaultKeyCtxKey{}, key), "again")
	if err != nil {
		t.Fatalf("reading the user with the vault's key: %v", err)
	}
	if usr.Onboarded {
		t.Error("onboarded already")
	}
}
