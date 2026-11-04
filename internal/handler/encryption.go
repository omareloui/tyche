package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/vault"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

const (
	vaultSessionCookieName = "vault_session"
	vaultPendingCookieName = "vault_pending"

	minVaultPasswordLen = 8
	maxVaultPasswordLen = 256

	pendingUnlockTTL = 10 * time.Minute

	// Failed unlocks before having to wait, then waiting twice as long after
	// each new one, up to maxUnlockWait.
	freeUnlockAttempts = 5
	firstUnlockWait    = 30 * time.Second
	maxUnlockWait      = time.Hour
)

var (
	pendingUnlocks = &pendingUnlockStore{logins: make(map[string]pendingUnlock)}
	unlockAttempts = &unlockLimiter{tenants: make(map[string]*unlockFailures)}
)

// pendingUnlock is a sign in to an encrypted database, waiting for the
// password: there's no user to sign in until the database is unlocked.
type pendingUnlock struct {
	provider models.OAuthProvider
	oauthID  string
	// gUsr is the google user, created once unlocked if the database has
	// none (its file was deleted, the vault is kept).
	gUsr    *models.User
	mobile  *mobileLogin
	expires time.Time
}

func (p *pendingUnlock) tenantID() string {
	return models.TenantID(p.provider, p.oauthID)
}

type pendingUnlockStore struct {
	mu     sync.Mutex
	logins map[string]pendingUnlock
}

func (s *pendingUnlockStore) add(p pendingUnlock) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, v := range s.logins {
		if now.After(v.expires) {
			delete(s.logins, k)
		}
	}
	p.expires = now.Add(pendingUnlockTTL)
	s.logins[id] = p
	return id, nil
}

func (s *pendingUnlockStore) get(id string) (*pendingUnlock, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.logins[id]
	if !ok || time.Now().After(p.expires) {
		return nil, false
	}
	return &p, true
}

func (s *pendingUnlockStore) remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.logins, id)
}

type unlockFailures struct {
	count int
	until time.Time
}

// unlockLimiter slows down guessing a tenant's password.
type unlockLimiter struct {
	mu      sync.Mutex
	tenants map[string]*unlockFailures
}

// wait is how long until the tenant can try again, 0 when they can now.
func (l *unlockLimiter) wait(tenantID string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, ok := l.tenants[tenantID]
	if !ok {
		return 0
	}
	return max(time.Until(f.until), 0)
}

func (l *unlockLimiter) fail(tenantID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, ok := l.tenants[tenantID]
	if !ok {
		f = &unlockFailures{}
		l.tenants[tenantID] = f
	}
	f.count++
	if over := f.count - freeUnlockAttempts; over >= 0 {
		wait := maxUnlockWait
		if over < 20 {
			wait = min(firstUnlockWait<<over, maxUnlockWait)
		}
		f.until = time.Now().Add(wait)
	}
}

func (l *unlockLimiter) reset(tenantID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.tenants, tenantID)
}

// forgetUnlocks drops the failed unlocks and pending sign ins of a deleted
// tenant.
func forgetUnlocks(tenantID string) {
	unlockAttempts.reset(tenantID)
	pendingUnlocks.mu.Lock()
	defer pendingUnlocks.mu.Unlock()
	for k, p := range pendingUnlocks.logins {
		if p.tenantID() == tenantID {
			delete(pendingUnlocks.logins, k)
		}
	}
}

// OpenVault is the middleware.VaultOpener: the key of the request's tenant
// database from its unlocked session cookie.
func (h *Handler) OpenVault(r *http.Request) ([]byte, bool, error) {
	tenantID, err := repository.TenantIDFromCtx(r.Context())
	if err != nil {
		return nil, false, nil
	}
	return h.vaultKey(r, tenantID)
}

// vaultKey is the tenant's key from the request's session cookie. encrypted
// is false when the tenant's database isn't, key is nil when it is but the
// request has no valid session for the tenant.
func (h *Handler) vaultKey(r *http.Request, tenantID string) (key []byte, encrypted bool, err error) {
	if !h.vaults.IsEncrypted(tenantID) {
		return nil, false, nil
	}
	c, err := r.Cookie(vaultSessionCookieName)
	if err != nil {
		return nil, true, nil
	}
	sessionTenant, key, err := h.vaults.OpenSession(r.Context(), c.Value)
	if errors.Is(err, vault.ErrInvalidSlot) || err == nil && sessionTenant != tenantID {
		return nil, true, nil
	}
	if err != nil {
		logger.FromCtx(r.Context()).Error("opening the vault session", zap.Error(err))
		return nil, true, err
	}
	return key, true, nil
}

// withVaultKey adds the tenant's key to the context when its database is
// encrypted, ErrTenantLocked when the request has no session to unlock it.
func (h *Handler) withVaultKey(ctx context.Context, r *http.Request, tenantID string) (context.Context, error) {
	key, encrypted, err := h.vaultKey(r, tenantID)
	if err != nil || !encrypted {
		return ctx, err
	}
	if key == nil {
		return ctx, repository.ErrTenantLocked
	}
	return context.WithValue(ctx, middleware.VaultKeyCtxKey{}, key), nil
}

// startPendingUnlock keeps the sign in until the user enters their password
// on the unlock page.
func (h *Handler) startPendingUnlock(w http.ResponseWriter, r *http.Request, p pendingUnlock) error {
	id, err := pendingUnlocks.add(p)
	if err != nil {
		return err
	}
	http.SetCookie(w, newCookie(vaultPendingCookieName, id, time.Now().Add(pendingUnlockTTL)))
	redirect(w, r, middleware.UnlockPath)
	return nil
}

// unlockTarget is the tenant the unlock page unlocks: the pending sign in's,
// else the signed in (or with a refresh token) user's.
func (h *Handler) unlockTarget(r *http.Request) (tenantID string, pending *pendingUnlock, pendingID string) {
	if c, err := r.Cookie(vaultPendingCookieName); err == nil {
		if p, ok := pendingUnlocks.get(c.Value); ok {
			return p.tenantID(), p, c.Value
		}
	}
	tenantID, err := repository.TenantIDFromCtx(r.Context())
	if err != nil {
		return "", nil, ""
	}
	return tenantID, nil, ""
}

func (h *Handler) UnlockPage(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	next := safeNext(r.URL.Query().Get("next"))

	tenantID, _, _ := h.unlockTarget(r)
	if tenantID == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return nil, nil
	}
	if !h.vaults.IsEncrypted(tenantID) {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return nil, nil
	}
	if key, _, err := h.vaultKey(r, tenantID); err == nil && key != nil && getClaims(r.Context()) != nil {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return nil, nil
	}

	return h.unlockView(r, next, "", tenantID), nil
}

func (h *Handler) Unlock(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())
	next := safeNext(r.FormValue("next"))

	tenantID, pending, pendingID := h.unlockTarget(r)
	if tenantID == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return nil, nil
	}

	if wait := unlockAttempts.wait(tenantID); wait > 0 {
		w.WriteHeader(http.StatusTooManyRequests)
		return h.unlockView(r, next, fmt.Sprintf("Too many wrong attempts, try again in %s.", roundWait(wait)), tenantID), nil
	}

	key, err := h.vaults.Unlock(r.Context(), tenantID, r.FormValue("password"))
	if errors.Is(err, vault.ErrNoVault) {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return nil, nil
	}
	if errors.Is(err, vault.ErrWrongSecret) {
		unlockAttempts.fail(tenantID)
		l.Warn("wrong vault password", zap.String("tenantID", tenantID))
		w.WriteHeader(http.StatusUnprocessableEntity)
		return h.unlockView(r, next, "Wrong password or recovery code.", tenantID), nil
	}
	if err != nil {
		return nil, err
	}
	unlockAttempts.reset(tenantID)

	to, err := h.finishUnlock(w, r, tenantID, pending, pendingID, key, next)
	if err != nil {
		return nil, err
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
	return nil, nil
}

// finishUnlock unlocks the database for the browser (or the mobile app of a
// pending sign in), signing in a pending sign in, and returns where to go
// next.
func (h *Handler) finishUnlock(w http.ResponseWriter, r *http.Request, tenantID string, pending *pendingUnlock, pendingID string, key []byte, next string) (string, error) {
	l := logger.FromCtx(r.Context())

	ctx := context.WithValue(r.Context(), middleware.VaultKeyCtxKey{}, key)
	var oauthID string
	if pending != nil {
		oauthID = pending.oauthID
		ctx = context.WithValue(ctx, middleware.AccessClaimsCtxKey{}, &jwt.AccessClaims{OAuthID: pending.oauthID, OAuthProvider: pending.provider})
		pendingUnlocks.remove(pendingID)
		unsetCookie(w, vaultPendingCookieName)
	} else {
		_, oauthID, _ = tenantOAuth(ctx)
	}

	// A sign in from the mobile app: its device gets its own slot.
	if pending != nil && pending.mobile != nil {
		usr, err := h.signInUser(ctx, pending.gUsr)
		if err != nil {
			return "", err
		}
		h.syncPicture(ctx, usr, pending.gUsr.Picture)
		code, err := mobileCodes.add(usr, pending.mobile, key)
		if err != nil {
			return "", err
		}
		l.Info("unlocked the database to sign in from mobile", zap.String("tenantID", tenantID))
		return mobileRedirectURI + "?code=" + url.QueryEscape(code), nil
	}

	if err := h.startVaultSession(ctx, w, r, tenantID, key); err != nil {
		return "", err
	}

	if pending != nil || getClaims(r.Context()) == nil {
		var usr *models.User
		var err error
		if pending != nil {
			usr, err = h.signInUser(ctx, pending.gUsr)
		} else {
			usr, err = h.repo.GetUserByOAuthID(ctx, oauthID)
		}
		if err != nil {
			return "", err
		}
		if pending != nil {
			h.syncPicture(ctx, usr, pending.gUsr.Picture)
			if !usr.Onboarded {
				next = "/welcome"
			}
		}
		if _, err := setJWTCookies(w, usr); err != nil {
			return "", err
		}
	}

	l.Info("unlocked the database", zap.String("tenantID", tenantID))
	return next, nil
}

// startVaultSession unlocks the database for the browser.
func (h *Handler) startVaultSession(ctx context.Context, w http.ResponseWriter, r *http.Request, tenantID string, key []byte) error {
	token, expires, err := h.vaults.NewSession(ctx, tenantID, key, r.UserAgent())
	if err != nil {
		return err
	}
	c := newCookie(vaultSessionCookieName, token, expires)
	c.SameSite = http.SameSiteLaxMode
	http.SetCookie(w, c)
	return nil
}

// endVaultSession locks the database again for the browser, also dropping a
// sign in waiting to unlock it.
func (h *Handler) endVaultSession(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(vaultPendingCookieName); err == nil {
		pendingUnlocks.remove(c.Value)
		unsetCookie(w, vaultPendingCookieName)
	}
	unsetCookie(w, vaultSessionCookieName)
	c, err := r.Cookie(vaultSessionCookieName)
	if err != nil {
		return
	}
	tenantID, err := repository.TenantIDFromCtx(ctx)
	if err != nil {
		return
	}
	if err := h.vaults.DeleteSession(ctx, tenantID, vault.SessionID(c.Value)); err != nil {
		logger.FromCtx(ctx).Error("deleting the vault session", zap.Error(err))
	}
}

// Security is the database encryption settings.
func (h *Handler) Security(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	data, err := h.securityData(r)
	if err != nil {
		return nil, err
	}
	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		return nil, err
	}
	return views.Security(getClaims(r.Context()), b, data), nil
}

func (h *Handler) securityData(r *http.Request) (*views.SecurityData, error) {
	tenantID, err := repository.TenantIDFromCtx(r.Context())
	if err != nil {
		return nil, err
	}
	data := &views.SecurityData{Encrypted: h.vaults.IsEncrypted(tenantID)}
	if !data.Encrypted {
		return data, nil
	}
	if data.Sessions, err = h.vaults.Sessions(r.Context(), tenantID); err != nil {
		return nil, err
	}
	if data.Passkeys, err = h.vaults.Passkeys(r.Context(), tenantID); err != nil {
		return nil, err
	}
	if c, err := r.Cookie(vaultSessionCookieName); err == nil {
		data.CurrentSessionID = vault.SessionID(c.Value)
	}
	return data, nil
}

// securityBody re-renders the settings in place after a change.
func (h *Handler) securityBody(w http.ResponseWriter, r *http.Request, data *views.SecurityData) templ.Component {
	w.Header().Set("HX-Retarget", "#security-body")
	w.Header().Set("HX-Reswap", "innerHTML")
	return views.SecurityBody(data)
}

func (h *Handler) EnableEncryption(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	password := r.FormValue("password")
	if err := validVaultPassword(password, r.FormValue("confirm")); err != nil {
		return nil, err
	}
	if r.FormValue("understood") == "" {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Confirm you understand a lost password and recovery code can't be recovered.")
	}

	key, code, err := h.repo.EnableEncryption(ctx, password)
	if err != nil {
		return nil, err
	}
	tenantID, err := repository.TenantIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, middleware.VaultKeyCtxKey{}, key)
	if err := h.startVaultSession(ctx, w, r, tenantID, key); err != nil {
		return nil, err
	}

	if r.FormValue("welcome") != "" {
		return h.welcomeStep(w, r, &views.WelcomeData{Step: models.OnboardingSecurity, Encrypted: true, RecoveryCode: code}), nil
	}

	data := &views.SecurityData{Encrypted: true, RecoveryCode: code, JustEnabled: true}
	if data.Sessions, err = h.vaults.Sessions(ctx, tenantID); err != nil {
		return nil, err
	}
	if len(data.Sessions) > 0 {
		data.CurrentSessionID = data.Sessions[0].ID
	}
	return h.securityBody(w, r, data), nil
}

func (h *Handler) DisableEncryption(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	key, err := h.checkVaultPassword(r, r.FormValue("password"))
	if err != nil {
		return nil, err
	}
	if err := h.repo.DisableEncryption(ctx, key); err != nil {
		return nil, err
	}
	unsetCookie(w, vaultSessionCookieName)
	return h.securityBody(w, r, &views.SecurityData{}), nil
}

func (h *Handler) ChangeVaultPassword(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	key, err := h.checkVaultPassword(r, r.FormValue("current"))
	if err != nil {
		return nil, err
	}
	password := r.FormValue("password")
	if err := validVaultPassword(password, r.FormValue("confirm")); err != nil {
		return nil, err
	}
	tenantID, err := repository.TenantIDFromCtx(r.Context())
	if err != nil {
		return nil, err
	}
	if err := h.vaults.ChangePassword(r.Context(), tenantID, key, password); err != nil {
		return nil, err
	}
	data, err := h.securityData(r)
	if err != nil {
		return nil, err
	}
	data.Notice = "Password changed."
	return h.securityBody(w, r, data), nil
}

func (h *Handler) RegenerateRecoveryCode(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	key, err := h.checkVaultPassword(r, r.FormValue("password"))
	if err != nil {
		return nil, err
	}
	tenantID, err := repository.TenantIDFromCtx(r.Context())
	if err != nil {
		return nil, err
	}
	code, err := h.vaults.RegenerateRecoveryCode(r.Context(), tenantID, key)
	if err != nil {
		return nil, err
	}
	data, err := h.securityData(r)
	if err != nil {
		return nil, err
	}
	data.RecoveryCode = code
	return h.securityBody(w, r, data), nil
}

// LockOtherSessions locks the database in every other browser.
func (h *Handler) LockOtherSessions(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	tenantID, err := repository.TenantIDFromCtx(r.Context())
	if err != nil {
		return nil, err
	}
	var keep string
	if c, err := r.Cookie(vaultSessionCookieName); err == nil {
		keep = vault.SessionID(c.Value)
	}
	if err := h.vaults.DeleteOtherSessions(r.Context(), tenantID, keep); err != nil {
		return nil, err
	}
	data, err := h.securityData(r)
	if err != nil {
		return nil, err
	}
	data.Notice = "Other browsers are locked."
	return h.securityBody(w, r, data), nil
}

// Lock locks the database in this browser.
func (h *Handler) Lock(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	h.endVaultSession(r.Context(), w, r)
	w.Header().Set("HX-Redirect", middleware.UnlockURL(middleware.CurrentURL(r)))
	return nil, nil
}

// checkVaultPassword gets the key with the password (or recovery code),
// limiting the attempts like unlocking.
func (h *Handler) checkVaultPassword(r *http.Request, password string) ([]byte, error) {
	tenantID, err := repository.TenantIDFromCtx(r.Context())
	if err != nil {
		return nil, err
	}
	if wait := unlockAttempts.wait(tenantID); wait > 0 {
		return nil, resperr.New(http.StatusTooManyRequests, fmt.Sprintf("Too many wrong attempts, try again in %s.", roundWait(wait)))
	}
	key, err := h.vaults.Unlock(r.Context(), tenantID, password)
	if errors.Is(err, vault.ErrWrongSecret) {
		unlockAttempts.fail(tenantID)
		return nil, resperr.New(http.StatusUnprocessableEntity, "Wrong password or recovery code.")
	}
	if errors.Is(err, vault.ErrNoVault) {
		return nil, resperr.New(http.StatusConflict, "The database isn't encrypted.")
	}
	if err != nil {
		return nil, err
	}
	unlockAttempts.reset(tenantID)
	return key, nil
}

func validVaultPassword(password, confirm string) error {
	if n := utf8.RuneCountInString(password); n < minVaultPasswordLen || n > maxVaultPasswordLen {
		return resperr.New(http.StatusUnprocessableEntity, fmt.Sprintf("The password needs %d to %d characters.", minVaultPasswordLen, maxVaultPasswordLen))
	}
	if password != confirm {
		return resperr.New(http.StatusUnprocessableEntity, "The passwords don't match.")
	}
	return nil
}

// safeNext keeps redirects on this site.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") || strings.HasPrefix(next, middleware.UnlockPath) {
		return "/"
	}
	return next
}

func roundWait(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()+0.5))
	}
	return fmt.Sprintf("%d minutes", int(d.Minutes()+0.5))
}

// tenantOAuth is who the tenant of the claims in the context is.
func tenantOAuth(ctx context.Context) (models.OAuthProvider, string, bool) {
	if c, ok := ctx.Value(middleware.AccessClaimsCtxKey{}).(*jwt.AccessClaims); ok && c != nil {
		return c.OAuthProvider, c.OAuthID, true
	}
	if c, ok := ctx.Value(middleware.RefreshClaimsCtxKey{}).(*jwt.RefreshClaims); ok && c != nil {
		return c.OAuthProvider, c.OAuthID, true
	}
	return 0, "", false
}
