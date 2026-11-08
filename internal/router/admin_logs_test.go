package router_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestAdminLogs(t *testing.T) {
	repo, _, admin := setupTenant(t)
	t.Setenv("ADMIN_TENANT_IDS", models.TenantID(admin.OAuthProvider, admin.OAuthID))
	plain := &models.User{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZB", OAuthID: "plain", OAuthProvider: models.Google}

	if err := os.MkdirAll("logs", 0o755); err != nil {
		t.Fatal(err)
	}
	logs := `{"level":"info","timestamp":"2026-11-08T20:58:35.640+0200","msg":"incoming request","correlation_id":"CID1","method":"GET","path":"/categories","status_code":200,"query":"month,q"}
{"level":"error","timestamp":"2026-11-08T20:58:36.640+0200","msg":"creating goal account","correlation_id":"CID2","error":"UNIQUE constraint failed"}
`
	if err := os.WriteFile("logs/tyche.log", []byte(logs), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)))
	defer srv.Close()

	get := func(path string, usr *models.User) (int, string, http.Header) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		req.Header.Set("HX-Request", "true")
		if usr != nil {
			pair, err := jwt.NewPair(usr)
			if err != nil {
				t.Fatal(err)
			}
			req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b), res.Header
	}

	for _, path := range []string{"/admin/logs", "/admin/logs/list", "/admin/debug/pprof/", "/admin/debug/pprof/heap"} {
		for _, usr := range []*models.User{nil, plain} {
			if code, _, _ := get(path, usr); code != http.StatusNotFound {
				t.Errorf("%s as %v: %d, want 404", path, usr, code)
			}
		}
	}

	code, body, _ := get("/admin/logs", admin)
	if code != http.StatusOK || !strings.Contains(body, "creating goal account") || !strings.Contains(body, "200 GET /categories ?month,q") || !strings.Contains(body, "Go heap") {
		t.Errorf("page: %d %s", code, body)
	}

	code, body, h := get("/admin/logs/list?level=error", admin)
	if code != http.StatusOK || !strings.Contains(body, "creating goal account") || strings.Contains(body, "/categories") {
		t.Errorf("error list: %d %s", code, body)
	}
	if got := h.Get("HX-Replace-Url"); got != "/admin/logs?level=error" {
		t.Errorf("HX-Replace-Url = %q", got)
	}

	if code, _, _ := get("/admin/logs/list?file=../data/x", admin); code != http.StatusNotFound {
		t.Errorf("unknown file: %d, want 404", code)
	}
	if code, body, _ := get("/admin/debug/pprof/heap?debug=1", admin); code != http.StatusOK || !strings.Contains(body, "heap profile") {
		t.Errorf("heap: %d", code)
	}
}
