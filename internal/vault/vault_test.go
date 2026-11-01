package vault

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/omareloui/tyche/internal/sharedqueries"
	_ "github.com/omareloui/tyche/internal/sqlite"
)

// testKDF keeps the tests fast, the derivation itself is argon2's.
var testKDF = KDFParams{Time: 1, Memory: 1024, Threads: 1}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "shared.sqlite")+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(db, "../../sql/shared/schema"); err != nil {
		t.Fatal(err)
	}
	return New(db, sharedqueries.New(db), testKDF)
}

func TestCreateUnlock(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if s.IsEncrypted("GL_1") {
		t.Fatal("encrypted before creating the vault")
	}
	if _, err := s.Unlock(ctx, "GL_1", "secret-pass"); !errors.Is(err, ErrNoVault) {
		t.Fatalf("unlocking without a vault: got %v, want ErrNoVault", err)
	}

	key, code, err := s.Create(ctx, "GL_1", "secret-pass")
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != keyLen {
		t.Fatalf("key length %d", len(key))
	}
	if !s.IsEncrypted("GL_1") {
		t.Fatal("not encrypted after creating the vault")
	}

	for _, secret := range []string{"secret-pass", code, strings.ToLower(strings.ReplaceAll(code, "-", " "))} {
		got, err := s.Unlock(ctx, "GL_1", secret)
		if err != nil {
			t.Fatalf("unlocking with %q: %v", secret, err)
		}
		if !bytes.Equal(got, key) {
			t.Fatalf("unlocking with %q: got another key", secret)
		}
	}

	for _, secret := range []string{"", "wrong-pass", strings.Repeat("A", len(normalizeRecoveryCode(code)))} {
		if _, err := s.Unlock(ctx, "GL_1", secret); !errors.Is(err, ErrWrongSecret) {
			t.Fatalf("unlocking with %q: got %v, want ErrWrongSecret", secret, err)
		}
	}

	// Another tenant's password doesn't open it, and Load reads the vaults.
	if _, _, err := s.Create(ctx, "GL_2", "other-pass"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Unlock(ctx, "GL_1", "other-pass"); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("got %v, want ErrWrongSecret", err)
	}
	fresh := New(s.db, s.q, testKDF)
	if err := fresh.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if !fresh.IsEncrypted("GL_1") || !fresh.IsEncrypted("GL_2") || fresh.IsEncrypted("GL_3") {
		t.Fatal("Load didn't read the vaults")
	}
}

func TestChangePasswordAndRecoveryCode(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	key, code, err := s.Create(ctx, "GL_1", "old-pass")
	if err != nil {
		t.Fatal(err)
	}

	if err := s.ChangePassword(ctx, "GL_1", key, "new-pass"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Unlock(ctx, "GL_1", "old-pass"); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("old password: got %v, want ErrWrongSecret", err)
	}
	if got, err := s.Unlock(ctx, "GL_1", "new-pass"); err != nil || !bytes.Equal(got, key) {
		t.Fatalf("new password: %v", err)
	}

	newCode, err := s.RegenerateRecoveryCode(ctx, "GL_1", key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Unlock(ctx, "GL_1", code); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("old code: got %v, want ErrWrongSecret", err)
	}
	if got, err := s.Unlock(ctx, "GL_1", newCode); err != nil || !bytes.Equal(got, key) {
		t.Fatalf("new code: %v", err)
	}
}

func TestSessions(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	key, _, err := s.Create(ctx, "GL_1", "pass-word")
	if err != nil {
		t.Fatal(err)
	}

	token, expires, err := s.NewSession(ctx, "GL_1", key, "Firefox")
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(expires) < SessionMaxAge-time.Minute {
		t.Fatalf("cookie expires too soon: %v", expires)
	}

	tenant, got, err := s.OpenSession(ctx, token)
	if err != nil || tenant != "GL_1" || !bytes.Equal(got, key) {
		t.Fatalf("opening the session: %q, %v", tenant, err)
	}

	id := SessionID(token)
	for _, bad := range []string{"", "nope", id + ".AAAA", id + "." + strings.Repeat("A", 43), "x" + token} {
		if _, _, err := s.OpenSession(ctx, bad); !errors.Is(err, ErrInvalidSlot) {
			t.Fatalf("opening %q: got %v, want ErrInvalidSlot", bad, err)
		}
	}

	other, _, err := s.NewSession(ctx, "GL_1", key, "Chrome")
	if err != nil {
		t.Fatal(err)
	}
	if sessions, err := s.Sessions(ctx, "GL_1"); err != nil || len(sessions) != 2 {
		t.Fatalf("got %d sessions (%v), want 2", len(sessions), err)
	}
	if err := s.DeleteOtherSessions(ctx, "GL_1", id); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.OpenSession(ctx, other); !errors.Is(err, ErrInvalidSlot) {
		t.Fatalf("other session: got %v, want ErrInvalidSlot", err)
	}

	// Another tenant can't delete it.
	if err := s.DeleteSession(ctx, "GL_2", id); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.OpenSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSession(ctx, "GL_1", id); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.OpenSession(ctx, token); !errors.Is(err, ErrInvalidSlot) {
		t.Fatalf("deleted session: got %v, want ErrInvalidSlot", err)
	}

	// An expired one is deleted when used.
	token, _, err = s.NewSession(ctx, "GL_1", key, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE vault_slots SET expires_at = ? WHERE id = ?", time.Now().UTC().Add(-time.Minute), SessionID(token)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.OpenSession(ctx, token); !errors.Is(err, ErrInvalidSlot) {
		t.Fatalf("expired session: got %v, want ErrInvalidSlot", err)
	}
	if sessions, _ := s.Sessions(ctx, "GL_1"); len(sessions) != 0 {
		t.Fatalf("got %d sessions, want 0", len(sessions))
	}

	// Deleting the vault deletes its slots.
	token, _, _ = s.NewSession(ctx, "GL_1", key, "")
	if err := s.Delete(ctx, "GL_1"); err != nil {
		t.Fatal(err)
	}
	if s.IsEncrypted("GL_1") {
		t.Fatal("still encrypted after deleting the vault")
	}
	if _, _, err := s.OpenSession(ctx, token); !errors.Is(err, ErrInvalidSlot) {
		t.Fatalf("session of a deleted vault: got %v, want ErrInvalidSlot", err)
	}
}

func TestDeviceSlots(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	key, _, err := s.Create(ctx, "GL_1", "pass-word")
	if err != nil {
		t.Fatal(err)
	}

	secret, err := s.NewDeviceSlot(ctx, "GL_1", "dev1", key)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.OpenDevice(ctx, "GL_1", "dev1", secret); err != nil || !bytes.Equal(got, key) {
		t.Fatalf("opening the device: %v", err)
	}
	if _, err := s.OpenDevice(ctx, "GL_1", "dev2", secret); !errors.Is(err, ErrInvalidSlot) {
		t.Fatalf("another device: got %v, want ErrInvalidSlot", err)
	}

	// Signing in again replaces the slot.
	renewed, err := s.NewDeviceSlot(ctx, "GL_1", "dev1", key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenDevice(ctx, "GL_1", "dev1", secret); !errors.Is(err, ErrInvalidSlot) {
		t.Fatalf("old secret: got %v, want ErrInvalidSlot", err)
	}
	if _, err := s.OpenDevice(ctx, "GL_1", "dev1", renewed); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteDevice(ctx, "GL_1", "dev1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenDevice(ctx, "GL_1", "dev1", renewed); !errors.Is(err, ErrInvalidSlot) {
		t.Fatalf("revoked device: got %v, want ErrInvalidSlot", err)
	}
}

func TestRecoveryCodeFormat(t *testing.T) {
	code, err := newRecoveryCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.Split(code, "-")) != 8 {
		t.Fatalf("code %q isn't 8 groups", code)
	}
	if !isRecoveryCode(normalizeRecoveryCode(code)) {
		t.Fatalf("code %q isn't read back", code)
	}
	if isRecoveryCode(normalizeRecoveryCode("my long password!")) {
		t.Fatal("a password read as a recovery code")
	}
}
