package router_test

import (
	"bytes"
	"database/sql"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestBackup(t *testing.T) {
	repo, ctx, usr := setupTenant(t)

	usd, err := repo.GetCurrencyByString(ctx, "USD")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.FirstName, usr.LastName, usr.DefaultCurrency = "e2e@example.com", "End", "ToEnd", *usd
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	kinds, err := repo.GetAccountKinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	newAccount := func(id, name string) {
		t.Helper()
		acc := &models.Account{ID: id, Name: name, Amount: 100, Currency: *usd, Kind: kinds[0]}
		if err := repo.CreateAccount(ctx, acc); err != nil {
			t.Fatal(err)
		}
	}
	newAccount("01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZB", "Before export")
	if err := repo.RegisterDevice(ctx, &models.Device{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZC", Name: "phone", InstallID: "i", TokenID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZD"}); err != nil {
		t.Fatal(err)
	}

	pair, err := jwt.NewPair(usr)
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "access_token", Value: pair.Access.Encoded}}
	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)))
	t.Cleanup(srv.Close)

	send := func(req *http.Request, htmx bool) (*http.Response, []byte) {
		t.Helper()
		if htmx {
			req.Header.Set("HX-Request", "true")
		}
		for _, c := range cookies {
			req.AddCookie(c)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, b
	}
	get := func(path string) (*http.Response, []byte) {
		t.Helper()
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		return send(req, false)
	}
	importFile := func(confirm string, file []byte, htmx bool) (*http.Response, string) {
		t.Helper()
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		_ = mw.WriteField("confirm", confirm)
		fw, _ := mw.CreateFormFile("file", "backup.sqlite")
		_, _ = fw.Write(file)
		mw.Close()
		req, _ := http.NewRequest("POST", srv.URL+"/backup/import", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		res, b := send(req, htmx)
		return res, string(b)
	}
	accounts := func() string {
		t.Helper()
		res, body := get("/accounts")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("accounts: got %d", res.StatusCode)
		}
		return string(body)
	}
	// tampered is the backup changed by the sql statements.
	tampered := func(t *testing.T, backup []byte, stmts ...string) []byte {
		t.Helper()
		path := filepath.Join(t.TempDir(), "b.sqlite")
		if err := os.WriteFile(path, backup, 0o600); err != nil {
			t.Fatal(err)
		}
		db, err := sql.Open("sqlite3", path)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range stmts {
			if _, err := db.Exec(s); err != nil {
				t.Fatalf("%s: %v", s, err)
			}
		}
		db.Close()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}

	var backup []byte
	t.Run("exports", func(t *testing.T) {
		res, body := get("/backup/export")
		if res.StatusCode != http.StatusOK || !strings.Contains(res.Header.Get("Content-Disposition"), "attachment") {
			t.Fatalf("got %d, %q", res.StatusCode, res.Header.Get("Content-Disposition"))
		}
		if !bytes.HasPrefix(body, []byte("SQLite format 3\x00")) {
			t.Fatal("not a plain sqlite database")
		}
		backup = body
		matches, _ := filepath.Glob("data/*.download")
		if len(matches) != 0 {
			t.Errorf("temp exports left: %v", matches)
		}
	})

	newAccount("01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZE", "After export")

	t.Run("rejects invalid imports", func(t *testing.T) {
		if res, _ := importFile(models.ImportConfirmation, backup, false); res.StatusCode != http.StatusForbidden {
			t.Errorf("without HX-Request: got %d", res.StatusCode)
		}
		if res, _ := importFile("nope", backup, true); res.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("unconfirmed: got %d", res.StatusCode)
		}
		cases := map[string][]byte{
			"garbage":      []byte("hello, not a database"),
			"newer":        tampered(t, backup, "INSERT INTO goose_db_version (version_id, is_applied) VALUES (99990101000000, 1)"),
			"trigger":      tampered(t, backup, "CREATE TRIGGER evil AFTER INSERT ON accounts BEGIN DELETE FROM transactions; END"),
			"no user":      tampered(t, backup, "DELETE FROM user"),
			"broken refs":  tampered(t, backup, "PRAGMA foreign_keys = OFF", "UPDATE accounts SET currency_id = 'missing'"),
			"no accounts":  tampered(t, backup, "DROP TABLE gamyaas", "DROP TABLE accounts"),
			"no migration": tampered(t, backup, "DROP TABLE goose_db_version"),
		}
		for name, file := range cases {
			if res, body := importFile(models.ImportConfirmation, file, true); res.StatusCode != http.StatusUnprocessableEntity {
				t.Errorf("%s: got %d: %s", name, res.StatusCode, body)
			}
		}
		if !strings.Contains(accounts(), "After export") {
			t.Fatal("data replaced by a rejected import")
		}
	})

	t.Run("imports", func(t *testing.T) {
		file := tampered(t, backup,
			"UPDATE user SET oauth_id = 'someone-else', email = 'other@example.com'",
			"INSERT INTO sms_messages (id, sender, body, received_at, hash, status, device_id) VALUES ('01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZF', 'HSBC', 'hi', '2026-11-01 10:00:00', 'h', 'ignored', '01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZC')",
		)
		res, body := importFile(models.ImportConfirmation, file, true)
		if res.StatusCode != http.StatusOK || res.Header.Get("HX-Redirect") != "/" {
			t.Fatalf("got %d: %s", res.StatusCode, body)
		}
		got := accounts()
		if !strings.Contains(got, "Before export") || strings.Contains(got, "After export") {
			t.Fatal("not the backup's accounts")
		}
		u, err := repo.GetUserByOAuthID(ctx, usr.OAuthID)
		if err != nil {
			t.Fatal(err)
		}
		if u.Email != usr.Email || u.ID != usr.ID {
			t.Errorf("identity from the backup: %+v", u)
		}
		devices, err := repo.GetDevices(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(devices) != 0 {
			t.Errorf("devices kept: %d", len(devices))
		}
	})

	t.Run("restores the previous data", func(t *testing.T) {
		if res, body := get("/backup"); res.StatusCode != http.StatusOK || !strings.Contains(string(body), "Restore previous data") {
			t.Fatalf("got %d", res.StatusCode)
		}
		req, _ := http.NewRequest("POST", srv.URL+"/backup/restore", nil)
		if res, body := send(req, true); res.StatusCode != http.StatusOK {
			t.Fatalf("got %d: %s", res.StatusCode, body)
		}
		if !strings.Contains(accounts(), "After export") {
			t.Fatal("previous data not restored")
		}
	})

	t.Run("re-encrypts imports of encrypted accounts", func(t *testing.T) {
		form := url.Values{"password": {"correct horse"}, "confirm": {"correct horse"}, "understood": {"1"}}
		req, _ := http.NewRequest("POST", srv.URL+"/security/encryption", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		res, body := send(req, true)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("encrypting: got %d: %s", res.StatusCode, body)
		}
		for _, c := range res.Cookies() {
			if c.Name == "vault_session" {
				cookies = append(cookies, c)
			}
		}
		if _, err := os.Stat("data/" + models.TenantID(usr.OAuthProvider, usr.OAuthID) + ".sqlite.bak"); !os.IsNotExist(err) {
			t.Error("backup with the old key kept after encrypting")
		}

		res, exported := get("/backup/export")
		if res.StatusCode != http.StatusOK || !bytes.HasPrefix(exported, []byte("SQLite format 3\x00")) {
			t.Fatalf("export: got %d, plain %v", res.StatusCode, bytes.HasPrefix(exported, []byte("SQLite format 3\x00")))
		}
		if res, body := importFile(models.ImportConfirmation, backup, true); res.StatusCode != http.StatusOK {
			t.Fatalf("import: got %d: %s", res.StatusCode, body)
		}
		raw, err := os.ReadFile("data/" + models.TenantID(usr.OAuthProvider, usr.OAuthID) + ".sqlite")
		if err != nil {
			t.Fatal(err)
		}
		if bytes.HasPrefix(raw, []byte("SQLite format 3")) {
			t.Fatal("imported database not encrypted")
		}
		got := accounts()
		if !strings.Contains(got, "Before export") || strings.Contains(got, "After export") {
			t.Fatal("not the backup's accounts")
		}
	})
}
