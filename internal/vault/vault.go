// Package vault keeps the keys of the encrypted tenant databases. A tenant
// database encrypted by its user (SQLCipher) has a random key (DEK), stored
// only wrapped in the shared database, once per slot: the user's password,
// their recovery code, every unlocked browser (session), every signed in
// mobile device and every passkey (fingerprint or face unlock). A slot's key
// is derived from a secret the server doesn't keep (Argon2id for the password
// and recovery code, HKDF for the random secrets of the sessions' cookies and
// the devices' tokens and for the passkeys' PRF outputs), so the database
// files and the shared database alone can't be read.
package vault

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/sharedqueries"
)

type Kind string

const (
	KindPassword Kind = "password"
	KindRecovery Kind = "recovery"
	KindSession  Kind = "session"
	KindDevice   Kind = "device"
	KindPasskey  Kind = "passkey"
)

const (
	// SessionIdleTTL is how long an unlocked browser stays unlocked unused.
	SessionIdleTTL = 12 * time.Hour
	// SessionMaxAge is how long it stays unlocked at most, as the refresh
	// token.
	SessionMaxAge = 30 * 24 * time.Hour
	// sessionTouchEvery limits how often using a session renews it.
	sessionTouchEvery = 5 * time.Minute
)

var (
	ErrWrongSecret = errors.New("vault: wrong password or recovery code")
	ErrInvalidSlot = errors.New("vault: invalid or expired slot")
	ErrNoVault     = errors.New("vault: the tenant isn't encrypted")
)

// Store holds the vaults in the shared database, and which tenants have one
// in memory (only this process writes them).
type Store struct {
	db  *sql.DB
	q   *sharedqueries.Queries
	kdf KDFParams

	mu        sync.RWMutex
	encrypted map[string]bool
}

func New(db *sql.DB, q *sharedqueries.Queries, kdf KDFParams) *Store {
	return &Store{db: db, q: q, kdf: kdf, encrypted: make(map[string]bool)}
}

// Load reads which tenants are encrypted.
func (s *Store) Load(ctx context.Context) error {
	ids, err := s.q.ListVaultTenants(ctx)
	if err != nil {
		return fmt.Errorf("vault: listing the encrypted tenants: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.encrypted = make(map[string]bool, len(ids))
	for _, id := range ids {
		s.encrypted[id] = true
	}
	return nil
}

func (s *Store) IsEncrypted(tenantID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.encrypted[tenantID]
}

func (s *Store) setEncrypted(tenantID string, encrypted bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if encrypted {
		s.encrypted[tenantID] = true
	} else {
		delete(s.encrypted, tenantID)
	}
}

// Create creates the tenant's vault with a new key, wrapped with the password
// and a new recovery code. The tenant counts as encrypted from now on: its
// database has to be encrypted with the key next (Delete on failure).
func (s *Store) Create(ctx context.Context, tenantID, password string) (key []byte, recoveryCode string, err error) {
	key, err = NewKey()
	if err != nil {
		return nil, "", err
	}
	recoveryCode, err = newRecoveryCode()
	if err != nil {
		return nil, "", err
	}

	err = s.transact(ctx, func(q *sharedqueries.Queries) error {
		if err := q.CreateVault(ctx, tenantID); err != nil {
			return err
		}
		if _, err := s.addPasswordSlot(ctx, q, tenantID, KindPassword, password, key); err != nil {
			return err
		}
		_, err := s.addPasswordSlot(ctx, q, tenantID, KindRecovery, normalizeRecoveryCode(recoveryCode), key)
		return err
	})
	if err != nil {
		return nil, "", fmt.Errorf("vault: creating: %w", err)
	}

	s.setEncrypted(tenantID, true)
	return key, recoveryCode, nil
}

// Info is an encrypted tenant's vault, as listed for the admins.
type Info struct {
	TenantID  string
	CreatedAt time.Time
	Sessions  int64
	Devices   int64
}

// List lists every vault, with how many browsers and devices it unlocks.
func (s *Store) List(ctx context.Context) ([]Info, error) {
	rows, err := s.q.ListVaults(ctx)
	if err != nil {
		return nil, fmt.Errorf("vault: listing: %w", err)
	}
	infos := make([]Info, len(rows))
	for i, r := range rows {
		infos[i] = Info{TenantID: r.TenantID, CreatedAt: r.CreatedAt, Sessions: r.Sessions, Devices: r.Devices}
	}
	return infos, nil
}

// Delete deletes the tenant's vault and all its slots, once its database is
// decrypted.
func (s *Store) Delete(ctx context.Context, tenantID string) error {
	if err := s.q.DeleteVault(ctx, tenantID); err != nil {
		return fmt.Errorf("vault: deleting: %w", err)
	}
	s.setEncrypted(tenantID, false)
	return nil
}

// Unlock gets the key with the password or the recovery code.
func (s *Store) Unlock(ctx context.Context, tenantID, secret string) ([]byte, error) {
	if !s.IsEncrypted(tenantID) {
		return nil, ErrNoVault
	}
	if secret == "" {
		return nil, ErrWrongSecret
	}

	if key, err := s.unlockKind(ctx, tenantID, KindPassword, secret); !errors.Is(err, ErrWrongSecret) {
		return key, err
	}
	if code := normalizeRecoveryCode(secret); isRecoveryCode(code) {
		return s.unlockKind(ctx, tenantID, KindRecovery, code)
	}
	return nil, ErrWrongSecret
}

func (s *Store) unlockKind(ctx context.Context, tenantID string, kind Kind, secret string) ([]byte, error) {
	slots, err := s.q.GetVaultSlotsByKind(ctx, sharedqueries.GetVaultSlotsByKindParams{TenantID: tenantID, Kind: string(kind)})
	if err != nil {
		return nil, fmt.Errorf("vault: getting the %s slots: %w", kind, err)
	}
	for _, slot := range slots {
		key, err := unwrapSlot(&slot, []byte(secret))
		if err == nil {
			return key, nil
		}
		if !errors.Is(err, errDecrypt) {
			return nil, err
		}
	}
	return nil, ErrWrongSecret
}

// ChangePassword wraps the key with a new password, the old one stops
// working.
func (s *Store) ChangePassword(ctx context.Context, tenantID string, key []byte, password string) error {
	return s.replacePasswordSlot(ctx, tenantID, KindPassword, password, key)
}

// RegenerateRecoveryCode replaces the recovery code.
func (s *Store) RegenerateRecoveryCode(ctx context.Context, tenantID string, key []byte) (string, error) {
	code, err := newRecoveryCode()
	if err != nil {
		return "", err
	}
	if err := s.replacePasswordSlot(ctx, tenantID, KindRecovery, normalizeRecoveryCode(code), key); err != nil {
		return "", err
	}
	return code, nil
}

func (s *Store) replacePasswordSlot(ctx context.Context, tenantID string, kind Kind, secret string, key []byte) error {
	return s.transact(ctx, func(q *sharedqueries.Queries) error {
		if err := q.DeleteVaultSlotsByKind(ctx, sharedqueries.DeleteVaultSlotsByKindParams{TenantID: tenantID, Kind: string(kind)}); err != nil {
			return err
		}
		_, err := s.addPasswordSlot(ctx, q, tenantID, kind, secret, key)
		return err
	})
}

func (s *Store) addPasswordSlot(ctx context.Context, q *sharedqueries.Queries, tenantID string, kind Kind, secret string, key []byte) (string, error) {
	id := idgen.New()
	w, err := wrapWithPassword(slotAAD(tenantID, kind, id), secret, key, s.kdf)
	if err != nil {
		return "", err
	}
	return id, s.insertSlot(ctx, q, tenantID, kind, id, w, slotOpts{})
}

type slotOpts struct {
	deviceID  string
	userAgent string
	expiresAt time.Time
}

func (s *Store) insertSlot(ctx context.Context, q *sharedqueries.Queries, tenantID string, kind Kind, id string, w *wrapped, o slotOpts) error {
	now := time.Now().UTC()
	params := sharedqueries.CreateVaultSlotParams{
		ID:         id,
		TenantID:   tenantID,
		Kind:       string(kind),
		Salt:       w.salt,
		WrappedKey: w.key,
		DeviceID:   sql.NullString{String: o.deviceID, Valid: o.deviceID != ""},
		UserAgent:  o.userAgent,
		ExpiresAt:  sql.NullTime{Time: o.expiresAt, Valid: !o.expiresAt.IsZero()},
		CreatedAt:  now,
	}
	if kind == KindSession {
		params.LastUsedAt = sql.NullTime{Time: now, Valid: true}
	}
	if w.kdf != nil {
		kdf, err := marshalKDF(w.kdf)
		if err != nil {
			return err
		}
		params.KdfParams = sql.NullString{String: kdf, Valid: true}
	}
	return q.CreateVaultSlot(ctx, params)
}

func unwrapSlot(slot *sharedqueries.VaultSlot, secret []byte) ([]byte, error) {
	w := &wrapped{salt: slot.Salt, key: slot.WrappedKey}
	if slot.KdfParams.Valid {
		kdf, err := unmarshalKDF(slot.KdfParams.String)
		if err != nil {
			return nil, err
		}
		w.kdf = kdf
	}
	return unwrap(slotAAD(slot.TenantID, Kind(slot.Kind), slot.ID), w, secret)
}

// Session is an unlocked browser.
type Session struct {
	ID         string
	UserAgent  string
	CreatedAt  time.Time
	LastUsedAt time.Time
	ExpiresAt  time.Time
}

// NewSession wraps the key for a browser, returning the token for its cookie
// and when the cookie should expire.
func (s *Store) NewSession(ctx context.Context, tenantID string, key []byte, userAgent string) (token string, expires time.Time, err error) {
	secret, err := randomBytes(secretLen)
	if err != nil {
		return "", time.Time{}, err
	}
	id := idgen.New()
	w, err := wrapWithSecret(slotAAD(tenantID, KindSession, id), secret, key)
	if err != nil {
		return "", time.Time{}, err
	}

	now := time.Now().UTC()
	if err := s.insertSlot(ctx, s.q, tenantID, KindSession, id, w, slotOpts{
		userAgent: truncate(userAgent, 256),
		expiresAt: now.Add(SessionIdleTTL),
	}); err != nil {
		return "", time.Time{}, fmt.Errorf("vault: creating a session: %w", err)
	}

	// Drops the expired ones, as they're only deleted when used otherwise.
	_, _ = s.Sessions(ctx, tenantID)

	return id + "." + base64.RawURLEncoding.EncodeToString(secret), now.Add(SessionMaxAge), nil
}

// OpenSession gets the tenant and key of a session's token, renewing it.
func (s *Store) OpenSession(ctx context.Context, token string) (tenantID string, key []byte, err error) {
	id, secret, ok := parseToken(token)
	if !ok {
		return "", nil, ErrInvalidSlot
	}
	slot, err := s.q.GetVaultSlot(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, ErrInvalidSlot
	}
	if err != nil {
		return "", nil, fmt.Errorf("vault: getting the session: %w", err)
	}
	if Kind(slot.Kind) != KindSession {
		return "", nil, ErrInvalidSlot
	}

	now := time.Now().UTC()
	if sessionExpired(&slot, now) {
		_ = s.q.DeleteVaultSlot(ctx, slot.ID)
		return "", nil, ErrInvalidSlot
	}

	key, err = unwrapSlot(&slot, secret)
	if err != nil {
		return "", nil, ErrInvalidSlot
	}

	if !slot.LastUsedAt.Valid || now.Sub(slot.LastUsedAt.Time) > sessionTouchEvery {
		expires := now.Add(SessionIdleTTL)
		if maxExpires := slot.CreatedAt.Add(SessionMaxAge); maxExpires.Before(expires) {
			expires = maxExpires
		}
		if err := s.q.TouchSessionSlot(ctx, sharedqueries.TouchSessionSlotParams{ID: slot.ID, Now: sql.NullTime{Time: now, Valid: true}, ExpiresAt: sql.NullTime{Time: expires, Valid: true}}); err != nil {
			return "", nil, fmt.Errorf("vault: renewing the session: %w", err)
		}
	}

	return slot.TenantID, key, nil
}

func sessionExpired(slot *sharedqueries.VaultSlot, now time.Time) bool {
	return !slot.ExpiresAt.Valid || !now.Before(slot.ExpiresAt.Time) || !now.Before(slot.CreatedAt.Add(SessionMaxAge))
}

// Sessions lists the tenant's unlocked browsers, newest first, deleting the
// expired ones.
func (s *Store) Sessions(ctx context.Context, tenantID string) ([]Session, error) {
	slots, err := s.q.GetVaultSlotsByKind(ctx, sharedqueries.GetVaultSlotsByKindParams{TenantID: tenantID, Kind: string(KindSession)})
	if err != nil {
		return nil, fmt.Errorf("vault: listing the sessions: %w", err)
	}

	now := time.Now().UTC()
	sessions := make([]Session, 0, len(slots))
	for _, slot := range slots {
		if sessionExpired(&slot, now) {
			_ = s.q.DeleteVaultSlot(ctx, slot.ID)
			continue
		}
		sessions = append(sessions, Session{
			ID:         slot.ID,
			UserAgent:  slot.UserAgent,
			CreatedAt:  slot.CreatedAt,
			LastUsedAt: slot.LastUsedAt.Time,
			ExpiresAt:  slot.ExpiresAt.Time,
		})
	}
	return sessions, nil
}

// SessionID is the id of a session's token, "" when it's not one.
func SessionID(token string) string {
	id, _, _ := parseToken(token)
	return id
}

// DeleteSession locks a browser again.
func (s *Store) DeleteSession(ctx context.Context, tenantID, id string) error {
	slot, err := s.q.GetVaultSlot(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if slot.TenantID != tenantID || Kind(slot.Kind) != KindSession {
		return nil
	}
	return s.q.DeleteVaultSlot(ctx, id)
}

// DeleteOtherSessions locks every browser of the tenant but the one with the
// keepID session.
func (s *Store) DeleteOtherSessions(ctx context.Context, tenantID, keepID string) error {
	return s.q.DeleteOtherSessionSlots(ctx, sharedqueries.DeleteOtherSessionSlotsParams{TenantID: tenantID, KeepID: keepID})
}

// NewDeviceSlot wraps the key for a mobile device, replacing its previous
// slot, and returns the secret for its token.
func (s *Store) NewDeviceSlot(ctx context.Context, tenantID, deviceID string, key []byte) (string, error) {
	secret, err := randomBytes(secretLen)
	if err != nil {
		return "", err
	}
	id := idgen.New()
	w, err := wrapWithSecret(slotAAD(tenantID, KindDevice, id), secret, key)
	if err != nil {
		return "", err
	}

	err = s.transact(ctx, func(q *sharedqueries.Queries) error {
		if err := q.DeleteDeviceVaultSlot(ctx, sharedqueries.DeleteDeviceVaultSlotParams{TenantID: tenantID, DeviceID: sql.NullString{String: deviceID, Valid: true}}); err != nil {
			return err
		}
		return s.insertSlot(ctx, q, tenantID, KindDevice, id, w, slotOpts{deviceID: deviceID})
	})
	if err != nil {
		return "", fmt.Errorf("vault: creating a device slot: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(secret), nil
}

// OpenDevice gets the key with a device's secret.
func (s *Store) OpenDevice(ctx context.Context, tenantID, deviceID, secret string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(secret)
	if err != nil || len(raw) != secretLen {
		return nil, ErrInvalidSlot
	}
	slot, err := s.q.GetDeviceVaultSlot(ctx, sharedqueries.GetDeviceVaultSlotParams{TenantID: tenantID, DeviceID: sql.NullString{String: deviceID, Valid: true}})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidSlot
	}
	if err != nil {
		return nil, fmt.Errorf("vault: getting the device slot: %w", err)
	}
	key, err := unwrapSlot(&slot, raw)
	if err != nil {
		return nil, ErrInvalidSlot
	}
	return key, nil
}

// DeleteDevice removes a device's slot, e.g. when it's revoked.
func (s *Store) DeleteDevice(ctx context.Context, tenantID, deviceID string) error {
	return s.q.DeleteDeviceVaultSlot(ctx, sharedqueries.DeleteDeviceVaultSlotParams{TenantID: tenantID, DeviceID: sql.NullString{String: deviceID, Valid: true}})
}

// DeleteDevices removes the slots of all the tenant's devices.
func (s *Store) DeleteDevices(ctx context.Context, tenantID string) error {
	return s.q.DeleteDeviceVaultSlots(ctx, tenantID)
}

func (s *Store) transact(ctx context.Context, fn func(*sharedqueries.Queries) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(s.q.WithTx(tx)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// parseToken splits a session's token, <slot id>.<secret>.
func parseToken(token string) (id string, secret []byte, ok bool) {
	id, enc, found := strings.Cut(token, ".")
	if !found || id == "" {
		return "", nil, false
	}
	secret, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil || len(secret) != secretLen {
		return "", nil, false
	}
	return id, secret, true
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
