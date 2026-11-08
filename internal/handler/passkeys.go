package handler

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/formatters"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/vault"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

// Passkeys unlock an encrypted database with the fingerprint, face or PIN of
// a device (WebAuthn, user verification required). The key is wrapped with
// the passkey's PRF output (the "prf" extension), which the browser sends
// once the assertion is made and the server never keeps, like a password.
// The password and recovery code still unlock it, e.g. on another device.

const (
	passkeyCeremonyTTL = 5 * time.Minute
	maxPasskeyBody     = 64 << 10
)

var passkeyCeremonies = &passkeyCeremonyStore{ceremonies: make(map[string]passkeyCeremony)}

type passkeyCeremony struct {
	tenantID string
	session  webauthn.SessionData
	// prfSalt is the new passkey's PRF input, registrations only.
	prfSalt []byte
	expires time.Time
}

type passkeyCeremonyStore struct {
	mu         sync.Mutex
	ceremonies map[string]passkeyCeremony
}

func (s *passkeyCeremonyStore) add(c passkeyCeremony) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, v := range s.ceremonies {
		if now.After(v.expires) {
			delete(s.ceremonies, k)
		}
	}
	c.expires = now.Add(passkeyCeremonyTTL)
	s.ceremonies[id] = c
	return id, nil
}

// pop removes the ceremony so its challenge is only answered once.
func (s *passkeyCeremonyStore) pop(id, tenantID string) (*passkeyCeremony, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.ceremonies[id]
	if !ok {
		return nil, false
	}
	delete(s.ceremonies, id)
	if time.Now().After(c.expires) || c.tenantID != tenantID {
		return nil, false
	}
	return &c, true
}

// relyingParty is the site the passkeys are for, OAUTH_CB_ORIGIN.
func relyingParty() (*webauthn.WebAuthn, error) {
	origin := config.GetOrigin()
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("passkeys: invalid origin %q", origin)
	}
	return webauthn.New(&webauthn.Config{
		RPID:          u.Hostname(),
		RPDisplayName: "Tyche",
		RPOrigins:     []string{strings.TrimSuffix(origin, "/")},
	})
}

// passkeyUser is the tenant as a WebAuthn user, with its passkeys.
type passkeyUser struct {
	tenantID string
	name     string
	creds    []webauthn.Credential
}

func (u *passkeyUser) WebAuthnID() []byte {
	sum := sha256.Sum256([]byte("tyche passkey user|" + u.tenantID))
	return sum[:]
}
func (u *passkeyUser) WebAuthnName() string                       { return u.name }
func (u *passkeyUser) WebAuthnDisplayName() string                { return u.name }
func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// passkeyUser loads the tenant's passkeys, also returned by their
// credential id.
func (h *Handler) passkeyUser(r *http.Request, tenantID string) (*passkeyUser, map[string]vault.Passkey, error) {
	passkeys, err := h.vaults.Passkeys(r.Context(), tenantID)
	if err != nil {
		return nil, nil, err
	}
	u := &passkeyUser{tenantID: tenantID, name: tenantID}
	if c := getClaims(r.Context()); c != nil && c.Email != "" {
		u.name = c.Email
	}
	byID := make(map[string]vault.Passkey, len(passkeys))
	for _, p := range passkeys {
		var cred webauthn.Credential
		if err := json.Unmarshal([]byte(p.Credential), &cred); err != nil {
			logger.FromCtx(r.Context()).Error("reading a passkey's credential", zap.String("slotID", p.ID), zap.Error(err))
			continue
		}
		u.creds = append(u.creds, cred)
		byID[string(p.CredentialID)] = p
	}
	return u, byID, nil
}

type passkeyOptionsResp struct {
	Ceremony  string `json:"ceremony"`
	PublicKey any    `json:"publicKey"`
	// PRF are the PRF inputs, by base64url credential id when unlocking
	// (evalByCredential), the new one's under "" when registering.
	PRF map[string]string `json:"prf"`
}

type passkeyFinishReq struct {
	Ceremony   string          `json:"ceremony"`
	Credential json.RawMessage `json:"credential"`
	// PRF is the base64url PRF output, kept out of the credential's client
	// extension results.
	PRF  string `json:"prf"`
	Next string `json:"next"`
}

func decodePasskeyFinish(w http.ResponseWriter, r *http.Request) (*passkeyFinishReq, []byte, error) {
	var req passkeyFinishReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPasskeyBody)).Decode(&req); err != nil {
		return nil, nil, resperr.New(http.StatusBadRequest, "Invalid request.")
	}
	prf, err := base64.RawURLEncoding.DecodeString(req.PRF)
	if err != nil || len(prf) == 0 {
		return nil, nil, resperr.New(http.StatusUnprocessableEntity, "This device can't unlock with a passkey (no PRF support), use the password.")
	}
	return &req, prf, nil
}

// PasskeyOptions starts adding a passkey, once the password is confirmed.
func (h *Handler) PasskeyOptions(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	tenantID, err := repository.TenantIDFromCtx(r.Context())
	if err != nil {
		return nil, err
	}
	if _, err := h.checkVaultPassword(r, r.FormValue("password")); err != nil {
		return nil, err
	}
	rp, err := relyingParty()
	if err != nil {
		return nil, err
	}
	u, _, err := h.passkeyUser(r, tenantID)
	if err != nil {
		return nil, err
	}

	exclude := make([]protocol.CredentialDescriptor, len(u.creds))
	for i, c := range u.creds {
		exclude[i] = c.Descriptor()
	}
	creation, session, err := rp.BeginRegistration(u,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			AuthenticatorAttachment: protocol.Platform,
			ResidentKey:             protocol.ResidentKeyRequirementPreferred,
			UserVerification:        protocol.VerificationRequired,
		}),
		webauthn.WithExclusions(exclude),
		webauthn.WithConveyancePreference(protocol.PreferNoAttestation),
	)
	if err != nil {
		return nil, err
	}
	salt, err := vault.NewPRFSalt()
	if err != nil {
		return nil, err
	}
	id, err := passkeyCeremonies.add(passkeyCeremony{tenantID: tenantID, session: *session, prfSalt: salt})
	if err != nil {
		return nil, err
	}
	return json.Marshal(passkeyOptionsResp{
		Ceremony:  id,
		PublicKey: creation.Response,
		PRF:       map[string]string{"": base64.RawURLEncoding.EncodeToString(salt)},
	})
}

// AddPasskey saves the passkey created for PasskeyOptions, its PRF output
// wrapping the key of the unlocked session.
func (h *Handler) AddPasskey(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	tenantID, err := repository.TenantIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	key, ok := ctx.Value(middleware.VaultKeyCtxKey{}).([]byte)
	if !ok || key == nil {
		return nil, resperr.New(http.StatusConflict, "The database isn't encrypted.")
	}
	req, prf, err := decodePasskeyFinish(w, r)
	if err != nil {
		return nil, err
	}
	c, ok := passkeyCeremonies.pop(req.Ceremony, tenantID)
	if !ok {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Took too long, try again.")
	}

	parsed, err := protocol.ParseCredentialCreationResponseBytes(req.Credential)
	if err != nil {
		logger.FromCtx(ctx).Warn("parsing a passkey", zap.Error(err))
		return nil, resperr.New(http.StatusUnprocessableEntity, "Couldn't read the passkey, try again.")
	}
	rp, err := relyingParty()
	if err != nil {
		return nil, err
	}
	u, _, err := h.passkeyUser(r, tenantID)
	if err != nil {
		return nil, err
	}
	cred, err := rp.CreateCredential(u, c.session, parsed)
	if err != nil {
		logger.FromCtx(ctx).Warn("verifying a new passkey", zap.Error(err))
		return nil, resperr.New(http.StatusUnprocessableEntity, "Couldn't verify the passkey, try again.")
	}
	credJSON, err := json.Marshal(cred)
	if err != nil {
		return nil, err
	}

	p := &vault.Passkey{
		CredentialID: cred.ID,
		Credential:   string(credJSON),
		PRFSalt:      c.prfSalt,
		Name:         formatters.UserAgent(r.UserAgent()),
	}
	if err := h.vaults.AddPasskey(ctx, tenantID, key, p, prf); err != nil {
		if errors.Is(err, vault.ErrInvalidSlot) {
			return nil, resperr.New(http.StatusUnprocessableEntity, "This device can't unlock with a passkey (no PRF support), use the password.")
		}
		return nil, err
	}
	logger.FromCtx(ctx).Info("added a passkey", zap.String("tenant_id", tenantID))
	return h.passkeysSection(r, tenantID, "")
}

// DeletePasskey stops a passkey from unlocking the database.
func (h *Handler) DeletePasskey(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	tenantID, err := repository.TenantIDFromCtx(r.Context())
	if err != nil {
		return nil, err
	}
	if err := h.vaults.DeletePasskey(r.Context(), tenantID, r.PathValue("id")); err != nil {
		return nil, err
	}
	return h.passkeysSection(r, tenantID, "Passkey removed.")
}

func (h *Handler) passkeysSection(r *http.Request, tenantID, notice string) (templ.Component, error) {
	passkeys, err := h.vaults.Passkeys(r.Context(), tenantID)
	if err != nil {
		return nil, err
	}
	return views.PasskeysSection(passkeys, notice), nil
}

// UnlockPasskeyOptions starts unlocking with a passkey, for the tenant of
// the unlock page.
func (h *Handler) UnlockPasskeyOptions(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	tenantID, _, _ := h.unlockTarget(r)
	if tenantID == "" || !h.vaults.IsEncrypted(tenantID) {
		return nil, resperr.New(http.StatusConflict, "Nothing to unlock.")
	}
	if wait := unlockAttempts.wait(tenantID); wait > 0 {
		return nil, resperr.New(http.StatusTooManyRequests, fmt.Sprintf("Too many wrong attempts, try again in %s.", roundWait(wait)))
	}
	rp, err := relyingParty()
	if err != nil {
		return nil, err
	}
	u, byID, err := h.passkeyUser(r, tenantID)
	if err != nil {
		return nil, err
	}
	if len(u.creds) == 0 {
		return nil, resperr.New(http.StatusConflict, "No passkey unlocks this account, use the password.")
	}

	assertion, session, err := rp.BeginLogin(u, webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return nil, err
	}
	id, err := passkeyCeremonies.add(passkeyCeremony{tenantID: tenantID, session: *session})
	if err != nil {
		return nil, err
	}
	prf := make(map[string]string, len(byID))
	for credID, p := range byID {
		prf[base64.RawURLEncoding.EncodeToString([]byte(credID))] = base64.RawURLEncoding.EncodeToString(p.PRFSalt)
	}
	return json.Marshal(passkeyOptionsResp{Ceremony: id, PublicKey: assertion.Response, PRF: prf})
}

type unlockPasskeyResp struct {
	Redirect string `json:"redirect"`
	// App is set when it's the mobile app's deep link, which the page may
	// need a tap to open.
	App bool `json:"app,omitempty"`
}

// UnlockPasskey unlocks with the passkey's assertion and PRF output, like
// Unlock with the password.
func (h *Handler) UnlockPasskey(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	l := logger.FromCtx(r.Context())
	tenantID, pending, pendingID := h.unlockTarget(r)
	if tenantID == "" || !h.vaults.IsEncrypted(tenantID) {
		return nil, resperr.New(http.StatusConflict, "Nothing to unlock.")
	}
	if wait := unlockAttempts.wait(tenantID); wait > 0 {
		return nil, resperr.New(http.StatusTooManyRequests, fmt.Sprintf("Too many wrong attempts, try again in %s.", roundWait(wait)))
	}
	req, prf, err := decodePasskeyFinish(w, r)
	if err != nil {
		return nil, err
	}
	next := safeNext(req.Next)
	c, ok := passkeyCeremonies.pop(req.Ceremony, tenantID)
	if !ok {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Took too long, try again.")
	}

	failed := func(msg string, err error) error {
		unlockAttempts.fail(tenantID)
		l.Warn("passkey unlock failed", zap.String("tenant_id", tenantID), zap.Error(err))
		return resperr.New(http.StatusUnprocessableEntity, msg)
	}

	parsed, err := protocol.ParseCredentialRequestResponseBytes(req.Credential)
	if err != nil {
		return nil, failed("Couldn't read the passkey, try again or use the password.", err)
	}
	rp, err := relyingParty()
	if err != nil {
		return nil, err
	}
	u, byID, err := h.passkeyUser(r, tenantID)
	if err != nil {
		return nil, err
	}
	cred, err := rp.ValidateLogin(u, c.session, parsed)
	if err != nil {
		return nil, failed("Couldn't verify the passkey, try again or use the password.", err)
	}
	if _, ok := byID[string(cred.ID)]; !ok || !bytes.Equal(cred.ID, parsed.RawID) {
		return nil, failed("Unknown passkey, use the password.", nil)
	}
	credJSON, err := json.Marshal(cred)
	if err != nil {
		return nil, err
	}
	key, err := h.vaults.OpenPasskey(r.Context(), tenantID, cred.ID, prf, string(credJSON))
	if errors.Is(err, vault.ErrWrongSecret) || errors.Is(err, vault.ErrInvalidSlot) {
		return nil, failed("This passkey doesn't unlock the database anymore, use the password.", err)
	}
	if err != nil {
		return nil, err
	}
	unlockAttempts.reset(tenantID)

	to, err := h.finishUnlock(w, r, tenantID, pending, pendingID, key, next)
	if err != nil {
		return nil, err
	}
	return json.Marshal(unlockPasskeyResp{Redirect: to, App: strings.HasPrefix(to, mobileRedirectURI)})
}

// unlockView is the unlock page, offering the passkeys when the tenant has
// some (the page checks the browser supports them).
func (h *Handler) unlockView(r *http.Request, next, errMsg, tenantID string) templ.Component {
	passkeys, err := h.vaults.Passkeys(r.Context(), tenantID)
	if err != nil {
		logger.FromCtx(r.Context()).Error("listing the passkeys", zap.Error(err))
	}
	return views.Unlock(next, errMsg, tenantID, len(passkeys) > 0)
}
