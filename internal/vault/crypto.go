package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	keyLen    = 32
	saltLen   = 16
	secretLen = 32

	recoveryCodeBytes = 20 // 32 base32 chars
	recoveryGroupLen  = 4

	hkdfInfo = "tyche vault slot"
)

// KDFParams are the Argon2id params deriving a password or recovery code's
// key, stored with its slot so they can be raised for new slots.
type KDFParams struct {
	Time    uint32 `json:"t"`
	Memory  uint32 `json:"m"` // KiB
	Threads uint8  `json:"p"`
}

// DefaultKDFParams are RFC 9106's second recommended option.
var DefaultKDFParams = KDFParams{Time: 3, Memory: 64 * 1024, Threads: 4}

var errDecrypt = errors.New("vault: can't decrypt the key")

// NewKey generates a database key (DEK).
func NewKey() ([]byte, error) {
	return randomBytes(keyLen)
}

// SQLCipherKey is the key as SQLCipher's raw key literal (x'…'), skipping its
// own key derivation as the key is random already.
func SQLCipherKey(key []byte) string {
	return "x'" + hex.EncodeToString(key) + "'"
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

func passwordKEK(password []byte, salt []byte, p KDFParams) []byte {
	return argon2.IDKey(password, salt, p.Time, p.Memory, p.Threads, keyLen)
}

func secretKEK(secret []byte, salt []byte) ([]byte, error) {
	return hkdf.Key(sha256.New, secret, salt, hkdfInfo, keyLen)
}

// slotAAD binds a wrapped key to its slot, so a row can't be moved to another
// tenant, kind or id.
func slotAAD(tenantID string, kind Kind, id string) []byte {
	return []byte(tenantID + "|" + string(kind) + "|" + id)
}

// seal encrypts the key with AES-256-GCM, the nonce prepended.
func seal(kek, key, aad []byte) ([]byte, error) {
	gcm, err := newGCM(kek)
	if err != nil {
		return nil, err
	}
	nonce, err := randomBytes(gcm.NonceSize())
	if err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, key, aad), nil
}

func open(kek, wrapped, aad []byte) ([]byte, error) {
	gcm, err := newGCM(kek)
	if err != nil {
		return nil, err
	}
	if len(wrapped) < gcm.NonceSize() {
		return nil, errDecrypt
	}
	nonce, ciphertext := wrapped[:gcm.NonceSize()], wrapped[gcm.NonceSize():]
	key, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, errDecrypt
	}
	return key, nil
}

func newGCM(kek []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// wrapped is a key wrapped for a slot.
type wrapped struct {
	salt []byte
	key  []byte
	kdf  *KDFParams
}

// wrapWithPassword wraps the key with a key derived from a password (or a
// recovery code) by Argon2id.
func wrapWithPassword(aad []byte, password string, key []byte, p KDFParams) (*wrapped, error) {
	salt, err := randomBytes(saltLen)
	if err != nil {
		return nil, err
	}
	sealed, err := seal(passwordKEK([]byte(password), salt, p), key, aad)
	if err != nil {
		return nil, err
	}
	return &wrapped{salt: salt, key: sealed, kdf: &p}, nil
}

// wrapWithSecret wraps the key with a key derived from a random secret (of a
// session or a device) by HKDF, cheap as the secret has full entropy.
func wrapWithSecret(aad []byte, secret []byte, key []byte) (*wrapped, error) {
	salt, err := randomBytes(saltLen)
	if err != nil {
		return nil, err
	}
	kek, err := secretKEK(secret, salt)
	if err != nil {
		return nil, err
	}
	sealed, err := seal(kek, key, aad)
	if err != nil {
		return nil, err
	}
	return &wrapped{salt: salt, key: sealed}, nil
}

// unwrap gets the key back with the slot's secret (a password, a recovery code
// or a random secret, depending on whether the slot has KDF params).
func unwrap(aad []byte, w *wrapped, secret []byte) ([]byte, error) {
	var kek []byte
	if w.kdf != nil {
		kek = passwordKEK(secret, w.salt, *w.kdf)
	} else {
		var err error
		if kek, err = secretKEK(secret, w.salt); err != nil {
			return nil, err
		}
	}
	return open(kek, w.key, aad)
}

func marshalKDF(p *KDFParams) (string, error) {
	b, err := json.Marshal(p)
	return string(b), err
}

func unmarshalKDF(s string) (*KDFParams, error) {
	var p KDFParams
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return nil, fmt.Errorf("vault: invalid kdf params: %w", err)
	}
	if p.Time == 0 || p.Memory == 0 || p.Threads == 0 {
		return nil, fmt.Errorf("vault: invalid kdf params %q", s)
	}
	return &p, nil
}

var recoveryEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// newRecoveryCode generates a recovery code, grouped for reading, e.g.
// ABCD-EFGH-….
func newRecoveryCode() (string, error) {
	b, err := randomBytes(recoveryCodeBytes)
	if err != nil {
		return "", err
	}
	code := recoveryEncoding.EncodeToString(b)
	groups := make([]string, 0, len(code)/recoveryGroupLen)
	for i := 0; i < len(code); i += recoveryGroupLen {
		groups = append(groups, code[i:min(i+recoveryGroupLen, len(code))])
	}
	return strings.Join(groups, "-"), nil
}

// normalizeRecoveryCode drops the grouping and case, so a code is read
// however it was typed. It's what the recovery slot's key is derived from.
func normalizeRecoveryCode(code string) string {
	code = strings.ToUpper(code)
	return strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' || r == '\t' || r == '\n' {
			return -1
		}
		return r
	}, code)
}

// isRecoveryCode reports whether the normalized code could be a recovery
// code, to skip the (slow) key derivation when it's a password.
func isRecoveryCode(normalized string) bool {
	if len(normalized) != recoveryEncoding.EncodedLen(recoveryCodeBytes) {
		return false
	}
	_, err := recoveryEncoding.DecodeString(normalized)
	return err == nil
}
