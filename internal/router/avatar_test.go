package router_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/omareloui/tyche/internal/avatar"
	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestAvatar(t *testing.T) {
	repo, ctx, usr := setupTenant(t)

	usd, err := repo.GetCurrencyByString(ctx, "USD")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.FirstName, usr.LastName, usr.DefaultCurrency = "avatar@example.com", "Ava", "Tar", *usd
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	pair, err := jwt.NewPair(usr)
	if err != nil {
		t.Fatal(err)
	}
	access := &http.Cookie{Name: "access_token", Value: pair.Access.Encoded}

	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t))))
	t.Cleanup(srv.Close)

	get := func(etag string) (*http.Response, []byte) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/avatar", nil)
		req.AddCookie(access)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, b
	}

	if res, _ := get(""); res.StatusCode != http.StatusNotFound {
		t.Fatalf("no picture: got %d, want 404", res.StatusCode)
	}

	pic := &avatar.Picture{Data: []byte("\x89PNG fake"), ContentType: "image/png", Hash: "0123456789abcdef"}
	if err := repo.SetUserPicture(ctx, "https://example.com/pic", pic); err != nil {
		t.Fatal(err)
	}

	res, body := get("")
	if res.StatusCode != http.StatusOK || !bytes.Equal(body, pic.Data) {
		t.Fatalf("got %d %q", res.StatusCode, body)
	}
	if ct := res.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("content type %q", ct)
	}
	etag := res.Header.Get("ETag")
	if etag != `"0123456789abcdef"` {
		t.Errorf("etag %q", etag)
	}
	if res, _ := get(etag); res.StatusCode != http.StatusNotModified {
		t.Errorf("if-none-match: got %d, want 304", res.StatusCode)
	}

	got, err := repo.GetUserByOAuthID(ctx, usr.OAuthID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AvatarURL() != "/avatar?v=0123456789abcdef" {
		t.Errorf("avatar url %q", got.AvatarURL())
	}
}
