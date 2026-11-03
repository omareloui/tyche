package vault

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestPasskeys(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	key, _, err := s.Create(ctx, "GL_1", "secret-pass")
	if err != nil {
		t.Fatal(err)
	}
	salt, err := NewPRFSalt()
	if err != nil {
		t.Fatal(err)
	}
	prf := bytes.Repeat([]byte{7}, prfLen)
	p := &Passkey{CredentialID: []byte("cred-1"), Credential: `{"v":1}`, PRFSalt: salt, Name: "Chrome on Android"}

	if err := s.AddPasskey(ctx, "GL_2", key, p, prf); !errors.Is(err, ErrNoVault) {
		t.Fatalf("adding to a tenant without a vault: got %v, want ErrNoVault", err)
	}
	if err := s.AddPasskey(ctx, "GL_1", key, p, prf[:16]); !errors.Is(err, ErrInvalidSlot) {
		t.Fatalf("adding with a short prf output: got %v, want ErrInvalidSlot", err)
	}
	if err := s.AddPasskey(ctx, "GL_1", key, p, prf); err != nil {
		t.Fatal(err)
	}

	list, err := s.Passkeys(ctx, "GL_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != p.ID || !bytes.Equal(list[0].PRFSalt, salt) || list[0].Name != p.Name || !list[0].LastUsedAt.IsZero() {
		t.Fatalf("listed %+v", list)
	}

	got, err := s.OpenPasskey(ctx, "GL_1", []byte("cred-1"), prf, `{"v":2}`)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, key) {
		t.Fatal("got another key")
	}
	if list, _ = s.Passkeys(ctx, "GL_1"); list[0].Credential != `{"v":2}` || list[0].LastUsedAt.IsZero() {
		t.Fatalf("not updated after use: %+v", list[0])
	}

	if _, err := s.OpenPasskey(ctx, "GL_1", []byte("cred-1"), bytes.Repeat([]byte{8}, prfLen), ""); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("opening with another prf output: got %v, want ErrWrongSecret", err)
	}
	if _, err := s.OpenPasskey(ctx, "GL_1", []byte("cred-2"), prf, ""); !errors.Is(err, ErrInvalidSlot) {
		t.Fatalf("opening an unknown credential: got %v, want ErrInvalidSlot", err)
	}
	if _, err := s.OpenPasskey(ctx, "GL_2", []byte("cred-1"), prf, ""); !errors.Is(err, ErrInvalidSlot) {
		t.Fatalf("opening another tenant's credential: got %v, want ErrInvalidSlot", err)
	}

	// Changing the password keeps the passkeys.
	if err := s.ChangePassword(ctx, "GL_1", key, "new-pass"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenPasskey(ctx, "GL_1", []byte("cred-1"), prf, `{}`); err != nil {
		t.Fatalf("opening after changing the password: %v", err)
	}

	// Only the tenant deletes its passkeys.
	if err := s.DeletePasskey(ctx, "GL_2", p.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.Passkeys(ctx, "GL_1"); len(list) != 1 {
		t.Fatal("another tenant deleted the passkey")
	}
	if err := s.DeletePasskey(ctx, "GL_1", p.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.Passkeys(ctx, "GL_1"); len(list) != 0 {
		t.Fatal("passkey not deleted")
	}

	// Deleting the vault (decrypting) drops them.
	if err := s.AddPasskey(ctx, "GL_1", key, &Passkey{CredentialID: []byte("cred-3"), PRFSalt: salt}, prf); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "GL_1"); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.Passkeys(ctx, "GL_1"); len(list) != 0 {
		t.Fatal("passkey kept after deleting the vault")
	}
}
