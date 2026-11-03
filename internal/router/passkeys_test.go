package router_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

const passkeyOrigin = "https://tyche.test"

var b64url = base64.RawURLEncoding

// softAuthenticator is a platform authenticator with one ES256 credential
// and a fixed PRF output, verifying the user every time.
type softAuthenticator struct {
	key    *ecdsa.PrivateKey
	credID []byte
	prf    []byte
	count  uint32
}

func newSoftAuthenticator(t *testing.T) *softAuthenticator {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a := &softAuthenticator{key: key, credID: make([]byte, 16), prf: make([]byte, 32)}
	rand.Read(a.credID)
	rand.Read(a.prf)
	return a
}

func (a *softAuthenticator) authData(attested []byte) []byte {
	rpHash := sha256.Sum256([]byte("tyche.test"))
	flags := byte(0x01 | 0x04) // user present + verified
	if attested != nil {
		flags |= 0x40
	}
	a.count++
	b := append(rpHash[:], flags)
	b = binary.BigEndian.AppendUint32(b, a.count)
	return append(b, attested...)
}

func clientData(t *testing.T, typ, challenge string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": passkeyOrigin})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// create answers navigator.credentials.create's options.
func (a *softAuthenticator) create(t *testing.T, challenge string) map[string]any {
	t.Helper()
	pub, err := webauthncbor.Marshal(webauthncose.EC2PublicKeyData{
		PublicKeyData: webauthncose.PublicKeyData{KeyType: 2, Algorithm: -7},
		Curve:         1,
		XCoord:        a.key.X.FillBytes(make([]byte, 32)),
		YCoord:        a.key.Y.FillBytes(make([]byte, 32)),
	})
	if err != nil {
		t.Fatal(err)
	}
	attested := make([]byte, 16) // aaguid
	attested = binary.BigEndian.AppendUint16(attested, uint16(len(a.credID)))
	attested = append(append(attested, a.credID...), pub...)
	att, err := webauthncbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": a.authData(attested)})
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"id": b64url.EncodeToString(a.credID), "rawId": b64url.EncodeToString(a.credID), "type": "public-key",
		"authenticatorAttachment": "platform",
		"response": map[string]any{
			"clientDataJSON":    b64url.EncodeToString(clientData(t, "webauthn.create", challenge)),
			"attestationObject": b64url.EncodeToString(att),
			"transports":        []string{"internal"},
		},
		"clientExtensionResults": map[string]any{},
	}
}

// get answers navigator.credentials.get's options.
func (a *softAuthenticator) get(t *testing.T, challenge string) map[string]any {
	t.Helper()
	auth := a.authData(nil)
	cd := clientData(t, "webauthn.get", challenge)
	hash := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, auth...), hash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"id": b64url.EncodeToString(a.credID), "rawId": b64url.EncodeToString(a.credID), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64url.EncodeToString(cd),
			"authenticatorData": b64url.EncodeToString(auth),
			"signature":         b64url.EncodeToString(sig),
		},
		"clientExtensionResults": map[string]any{},
	}
}

func TestPasskeys(t *testing.T) {
	repo, ctx, usr := setupTenant(t)
	t.Setenv("OAUTH_CB_ORIGIN", passkeyOrigin)

	usd, err := repo.GetCurrencyByString(ctx, "USD")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.FirstName, usr.DefaultCurrency = "passkey@example.com", "Pass", *usd
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	pair, err := jwt.NewPair(usr)
	if err != nil {
		t.Fatal(err)
	}
	access := &http.Cookie{Name: "access_token", Value: pair.Access.Encoded}
	refresh := &http.Cookie{Name: "refresh_token", Value: pair.Refresh.Encoded}
	tenantID := models.TenantID(usr.OAuthProvider, usr.OAuthID)

	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t))))
	t.Cleanup(srv.Close)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	do := func(path string, body any, cookies ...*http.Cookie) (*http.Response, string) {
		t.Helper()
		var r io.Reader
		ct := "application/json"
		switch b := body.(type) {
		case url.Values:
			r, ct = strings.NewReader(b.Encode()), "application/x-www-form-urlencoded"
		default:
			j, _ := json.Marshal(b)
			r = bytes.NewReader(j)
		}
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, r)
		req.Header.Set("Content-Type", ct)
		req.Header.Set("HX-Request", "true")
		for _, c := range cookies {
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
	get := func(path string, cookies ...*http.Cookie) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		for _, c := range cookies {
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
	type options struct {
		Ceremony  string            `json:"ceremony"`
		PRF       map[string]string `json:"prf"`
		PublicKey struct {
			Challenge        string `json:"challenge"`
			AllowCredentials []struct {
				ID string `json:"id"`
			} `json:"allowCredentials"`
			AuthenticatorSelection struct {
				UserVerification string `json:"userVerification"`
			} `json:"authenticatorSelection"`
		} `json:"publicKey"`
	}
	readOptions := func(res *http.Response, body string) options {
		t.Helper()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("options: got %d: %s", res.StatusCode, body)
		}
		var o options
		if err := json.Unmarshal([]byte(body), &o); err != nil {
			t.Fatal(err)
		}
		return o
	}

	res, body := do("/security/encryption", url.Values{"password": {"correct horse"}, "confirm": {"correct horse"}, "understood": {"1"}, "passkey": {"1"}}, access)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "data-passkey-enroll") {
		t.Fatalf("encrypting: got %d", res.StatusCode)
	}
	session := cookie(res, "vault_session")

	auth := newSoftAuthenticator(t)

	t.Run("adds a passkey with the password", func(t *testing.T) {
		if res, _ := do("/security/passkeys/options", url.Values{"password": {"wrong horse"}}, access, session); res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("wrong password: got %d", res.StatusCode)
		}
		o := readOptions(do("/security/passkeys/options", url.Values{"password": {"correct horse"}}, access, session))
		if o.PublicKey.AuthenticatorSelection.UserVerification != "required" || len(o.PRF[""]) == 0 {
			t.Fatalf("options %+v", o)
		}

		if res, _ := do("/security/passkeys", map[string]any{"ceremony": o.Ceremony, "credential": auth.create(t, o.PublicKey.Challenge)}, access, session); res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("without a prf output: got %d", res.StatusCode)
		}
		// The failed answer used the ceremony up.
		o = readOptions(do("/security/passkeys/options", url.Values{"password": {"correct horse"}}, access, session))
		if res, _ := do("/security/passkeys", map[string]any{"ceremony": o.Ceremony, "credential": auth.create(t, "bm90LXRoZS1jaGFsbGVuZ2U"), "prf": b64url.EncodeToString(auth.prf)}, access, session); res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("another challenge: got %d", res.StatusCode)
		}
		o = readOptions(do("/security/passkeys/options", url.Values{"password": {"correct horse"}}, access, session))
		res, body := do("/security/passkeys", map[string]any{"ceremony": o.Ceremony, "credential": auth.create(t, o.PublicKey.Challenge), "prf": b64url.EncodeToString(auth.prf)}, access, session)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, `id="passkeys"`) || !strings.Contains(body, "/delete") {
			t.Fatalf("got %d: %s", res.StatusCode, body)
		}
		if _, body := get("/security", access, session); !strings.Contains(body, `id="passkeys"`) || !strings.Contains(body, "/security/passkeys/") {
			t.Fatal("security page doesn't list it")
		}
	})

	unlock := func(prf []byte, next string, cookies ...*http.Cookie) (*http.Response, string) {
		t.Helper()
		o := readOptions(do("/unlock/passkey/options", url.Values{}, cookies...))
		if len(o.PublicKey.AllowCredentials) != 1 || o.PublicKey.AllowCredentials[0].ID != b64url.EncodeToString(auth.credID) || o.PRF[b64url.EncodeToString(auth.credID)] == "" {
			t.Fatalf("unlock options %+v", o)
		}
		return do("/unlock/passkey", map[string]any{"ceremony": o.Ceremony, "credential": auth.get(t, o.PublicKey.Challenge), "prf": b64url.EncodeToString(prf), "next": next}, cookies...)
	}

	t.Run("unlocks with it", func(t *testing.T) {
		if _, body := get("/unlock", access); !strings.Contains(body, "Unlock with a passkey") {
			t.Fatal("the unlock page doesn't offer it")
		}

		if res, _ := unlock(bytes.Repeat([]byte{1}, 32), "/accounts", access); res.StatusCode != http.StatusUnprocessableEntity || cookie(res, "vault_session") != nil {
			t.Fatalf("another prf output: got %d", res.StatusCode)
		}

		res, body := unlock(auth.prf, "/accounts", access)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, `"redirect":"/accounts"`) {
			t.Fatalf("got %d: %s", res.StatusCode, body)
		}
		s := cookie(res, "vault_session")
		if s == nil {
			t.Fatal("no vault session")
		}
		if res, _ := get("/accounts", access, s); res.StatusCode != http.StatusOK {
			t.Fatalf("reading with its session: got %d", res.StatusCode)
		}

		// Only the refresh token: signs in too.
		res, _ = unlock(auth.prf, "//evil.example", refresh)
		if res.StatusCode != http.StatusOK || cookie(res, "access_token") == nil || cookie(res, "vault_session") == nil {
			t.Fatalf("with the refresh token: got %d", res.StatusCode)
		}

		// The password still unlocks, e.g. on another device.
		if res, _ := do("/unlock", url.Values{"password": {"correct horse"}}, access); res.StatusCode != http.StatusSeeOther || cookie(res, "vault_session") == nil {
			t.Fatalf("password: got %d", res.StatusCode)
		}
	})

	t.Run("a ceremony is answered once", func(t *testing.T) {
		o := readOptions(do("/unlock/passkey/options", url.Values{}, access))
		answer := map[string]any{"ceremony": o.Ceremony, "credential": auth.get(t, o.PublicKey.Challenge), "prf": b64url.EncodeToString(auth.prf)}
		if res, _ := do("/unlock/passkey", answer, access); res.StatusCode != http.StatusOK {
			t.Fatalf("first: got %d", res.StatusCode)
		}
		if res, _ := do("/unlock/passkey", answer, access); res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("replayed: got %d", res.StatusCode)
		}
	})

	t.Run("removes it", func(t *testing.T) {
		passkeys, err := repo.Vaults().Passkeys(ctx, tenantID)
		if err != nil || len(passkeys) != 1 {
			t.Fatalf("passkeys %v %v", passkeys, err)
		}
		if res, _ := do("/security/passkeys/"+passkeys[0].ID+"/delete", url.Values{}, access, session); res.StatusCode != http.StatusOK {
			t.Fatalf("got %d", res.StatusCode)
		}
		if res, _ := do("/unlock/passkey/options", url.Values{}, access); res.StatusCode != http.StatusConflict {
			t.Fatalf("options without passkeys: got %d", res.StatusCode)
		}
		if _, body := get("/unlock", access); strings.Contains(body, "Unlock with a passkey") {
			t.Fatal("the unlock page still offers it")
		}
	})
}
