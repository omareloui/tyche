package router_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestPWA(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	repo, ctx, usr := setupTenant(t)
	usd, err := repo.GetCurrencyByString(ctx, "USD")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.FirstName, usr.LastName, usr.DefaultCurrency = "pwa@example.com", "Pwa", "User", *usd
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
	// Static files are served from ./web/public, opened on each request.
	t.Chdir(root)

	get := func(path string, cookie *http.Cookie) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}

	t.Run("manifest", func(t *testing.T) {
		res, body := get("/manifest.webmanifest", nil)
		if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/manifest+json" {
			t.Fatalf("got %d %q", res.StatusCode, res.Header.Get("Content-Type"))
		}
		var m struct {
			Icons       []struct{ Src string } `json:"icons"`
			ShareTarget struct {
				Action string `json:"action"`
				Method string `json:"method"`
			} `json:"share_target"`
		}
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			t.Fatal(err)
		}
		if m.ShareTarget.Action != "/share-receipt" || m.ShareTarget.Method != "POST" {
			t.Errorf("share_target: got %+v", m.ShareTarget)
		}
		if !strings.Contains(swBody(t), `"`+m.ShareTarget.Action+`"`) {
			t.Error("sw.js doesn't take the share_target's posts")
		}
		for _, ic := range m.Icons {
			if res, _ := get(ic.Src, nil); res.StatusCode != http.StatusOK {
				t.Errorf("icon %s: got %d", ic.Src, res.StatusCode)
			}
		}
	})

	t.Run("service worker and its precache", func(t *testing.T) {
		res, body := get("/sw.js", nil)
		if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/javascript") {
			t.Fatalf("got %d %q", res.StatusCode, res.Header.Get("Content-Type"))
		}
		// A missing url fails the whole install.
		list := regexp.MustCompile(`(?s)const PRECACHE = \[(.*?)\];`).FindStringSubmatch(body)
		if list == nil {
			t.Fatal("no PRECACHE in sw.js")
		}
		urls := regexp.MustCompile(`"(/[^"]+)"`).FindAllStringSubmatch(list[1], -1)
		if len(urls) == 0 {
			t.Fatal("empty PRECACHE")
		}
		for _, u := range urls {
			if res, _ := get(u[1], nil); res.StatusCode != http.StatusOK {
				t.Errorf("precached %s: got %d", u[1], res.StatusCode)
			}
		}
	})

	t.Run("a share without a service worker opens the create form", func(t *testing.T) {
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		res, err := client.Post(srv.URL+"/share-receipt", "multipart/form-data; boundary=x", strings.NewReader("--x--\r\n"))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/transactions?new" {
			t.Fatalf("got %d %q", res.StatusCode, res.Header.Get("Location"))
		}
	})

	t.Run("offline page never shows the user", func(t *testing.T) {
		for _, c := range []*http.Cookie{nil, access} {
			res, body := get("/offline", c)
			if res.StatusCode != http.StatusOK || !strings.Contains(body, "You're offline") {
				t.Fatalf("cookie %v: got %d", c != nil, res.StatusCode)
			}
			if strings.Contains(body, usr.Email) || strings.Contains(body, "Sign in") {
				t.Fatalf("cookie %v: the offline page has user/sign in content", c != nil)
			}
		}
	})

	t.Run("the layout links the manifest", func(t *testing.T) {
		_, body := get("/", access)
		if !strings.Contains(body, `rel="manifest" href="/manifest.webmanifest"`) {
			t.Fatal("no manifest link")
		}
	})
}

func swBody(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("web/public/sw.js")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
