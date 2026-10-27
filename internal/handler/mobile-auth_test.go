package handler

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/omareloui/tyche/internal/models"
)

func TestPKCE(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	if !isPKCEChallenge(challenge) {
		t.Fatal("valid challenge rejected")
	}
	if isPKCEChallenge("short") || isPKCEChallenge(challenge[:42]+"=") {
		t.Error("invalid challenge accepted")
	}
	if !verifyPKCE(verifier, challenge) {
		t.Error("valid verifier rejected")
	}
	if verifyPKCE("", challenge) || verifyPKCE(verifier+"x", challenge) {
		t.Error("invalid verifier accepted")
	}
}

func TestMobileCodeIsSingleUse(t *testing.T) {
	store := &mobileCodeStore{codes: make(map[string]mobileCode)}
	code, err := store.add(&models.User{ID: "u"}, &mobileLogin{challenge: "c", deviceName: "d"})
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := store.pop(code); !ok || c.usr.ID != "u" || c.login.challenge != "c" {
		t.Fatalf("expected the code, got %+v %v", c, ok)
	}
	if _, ok := store.pop(code); ok {
		t.Error("code got used twice")
	}
}

func TestOAuthStateStore(t *testing.T) {
	store := &oauthStateStore{states: make(map[string]oauthState)}
	store.add("web", nil)
	store.add("mobile", &mobileLogin{challenge: "c"})

	if m, ok := store.pop("web"); !ok || m != nil {
		t.Errorf("web state: got %+v %v", m, ok)
	}
	if m, ok := store.pop("mobile"); !ok || m == nil || m.challenge != "c" {
		t.Errorf("mobile state: got %+v %v", m, ok)
	}
	if _, ok := store.pop("mobile"); ok {
		t.Error("state got used twice")
	}
}

func TestDeviceName(t *testing.T) {
	if got := deviceName("  "); got != defaultDeviceName {
		t.Errorf("got %q", got)
	}
	long := ""
	for range 100 {
		long += "ب"
	}
	if got := []rune(deviceName(long)); len(got) != maxDeviceNameLen {
		t.Errorf("got %d runes", len(got))
	}
}
