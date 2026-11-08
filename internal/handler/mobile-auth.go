package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"go.uber.org/zap"
)

const (
	mobileRedirectURI = "tyche://auth"

	oauthStateTTL      = 10 * time.Minute
	mobileCodeTTL      = 2 * time.Minute
	maxDeviceNameLen   = 64
	maxInstallIDLen    = 64
	defaultDeviceName  = "Android"
	pkceChallengeLen   = 43 // base64url(sha256) without padding
	mobileCodeEntropyB = 32
)

var (
	oauthStates = &oauthStateStore{states: make(map[string]oauthState)}
	mobileCodes = &mobileCodeStore{codes: make(map[string]mobileCode)}
)

// mobileLogin is the mobile app's sign in request, kept until the oauth
// callback.
type mobileLogin struct {
	challenge  string
	deviceName string
	installID  string
}

type oauthState struct {
	mobile  *mobileLogin
	expires time.Time
}

type oauthStateStore struct {
	mu     sync.Mutex
	states map[string]oauthState
}

func (s *oauthStateStore) add(state string, mobile *mobileLogin) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, v := range s.states {
		if now.After(v.expires) {
			delete(s.states, k)
		}
	}
	s.states[state] = oauthState{mobile: mobile, expires: now.Add(oauthStateTTL)}
}

// peek is pop without removing the state.
func (s *oauthStateStore) peek(state string) (*mobileLogin, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.states[state]
	if !ok || time.Now().After(st.expires) {
		return nil, false
	}
	return st.mobile, true
}

// pop removes the state and returns its mobile login (nil for web sign ins).
func (s *oauthStateStore) pop(state string) (*mobileLogin, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.states[state]
	if !ok {
		return nil, false
	}
	delete(s.states, state)
	if time.Now().After(st.expires) {
		return nil, false
	}
	return st.mobile, true
}

type mobileCode struct {
	usr   *models.User
	login mobileLogin
	// key is the user's database key when it's encrypted, for the device's
	// vault slot.
	key     []byte
	expires time.Time
}

type mobileCodeStore struct {
	mu    sync.Mutex
	codes map[string]mobileCode
}

func (s *mobileCodeStore) add(usr *models.User, login *mobileLogin, key []byte) (string, error) {
	b := make([]byte, mobileCodeEntropyB)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	code := base64.RawURLEncoding.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, v := range s.codes {
		if now.After(v.expires) {
			delete(s.codes, k)
		}
	}
	s.codes[code] = mobileCode{usr: usr, login: *login, key: key, expires: now.Add(mobileCodeTTL)}
	return code, nil
}

// pop removes the code so it can only be used once.
func (s *mobileCodeStore) pop(code string) (*mobileCode, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.codes[code]
	if !ok {
		return nil, false
	}
	delete(s.codes, code)
	if time.Now().After(c.expires) {
		return nil, false
	}
	return &c, true
}

// OAuthMobile starts the sign in from the mobile app. The app passes a PKCE
// challenge, gets a one time code on its deep link after signing in, then
// exchanges it with the verifier for a device token in MobileToken.
func (h *Handler) OAuthMobile(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	challenge := r.URL.Query().Get("challenge")
	if !isPKCEChallenge(challenge) {
		return nil, resperr.New(http.StatusBadRequest, "invalid challenge")
	}

	// Optional, older app versions don't send it.
	installID := r.URL.Query().Get("install_id")
	if installID != "" && !isInstallID(installID) {
		return nil, resperr.New(http.StatusBadRequest, "invalid install id")
	}

	csrfId := idgen.New()
	oauthStates.add(csrfId, &mobileLogin{
		challenge:  challenge,
		deviceName: deviceName(r.URL.Query().Get("device_name")),
		installID:  installID,
	})
	http.Redirect(w, r, getOAuth2Conf().AuthCodeURL(csrfId), http.StatusTemporaryRedirect)
	return nil, nil
}

type mobileTokenReq struct {
	Code     string `json:"code"`
	Verifier string `json:"verifier"`
}

type mobileTokenResp struct {
	Token    string `json:"token"`
	DeviceID string `json:"device_id"`
}

func (h *Handler) MobileToken(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	l := logger.FromCtx(r.Context())

	var req mobileTokenReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "invalid json body")
	}

	code, ok := mobileCodes.pop(req.Code)
	if !ok || !verifyPKCE(req.Verifier, code.login.challenge) {
		l.Warn("invalid mobile code or verifier")
		return nil, resperr.New(http.StatusUnauthorized, "invalid code")
	}

	ctx := context.WithValue(r.Context(), middleware.AccessClaimsCtxKey{}, &jwt.AccessClaims{
		ID:            code.usr.ID,
		OAuthID:       code.usr.OAuthID,
		OAuthProvider: code.usr.OAuthProvider,
	})
	if code.key != nil {
		ctx = context.WithValue(ctx, middleware.VaultKeyCtxKey{}, code.key)
	}

	device := &models.Device{
		ID:        idgen.New(),
		Name:      code.login.deviceName,
		InstallID: code.login.installID,
		TokenID:   idgen.New(),
	}
	if err := h.repo.RegisterDevice(ctx, device); err != nil {
		return nil, err
	}

	// The device unlocks the encrypted database with its own slot, to send
	// sms while the user isn't signed in.
	var vaultSecret string
	if code.key != nil {
		tenantID := models.TenantID(code.usr.OAuthProvider, code.usr.OAuthID)
		var err error
		if vaultSecret, err = h.vaults.NewDeviceSlot(ctx, tenantID, device.ID, code.key); err != nil {
			l.Error("creating the device vault slot", zap.Error(err))
			return nil, err
		}
	}

	token, err := jwt.NewDeviceToken(code.usr, device.ID, device.TokenID, vaultSecret)
	if err != nil {
		l.Error("generating device token", zap.Error(err))
		return nil, err
	}

	l.Info("registered mobile device", zap.String("device_id", device.ID), zap.String("tenant_id", models.TenantID(code.usr.OAuthProvider, code.usr.OAuthID)))
	return json.Marshal(mobileTokenResp{Token: token, DeviceID: device.ID})
}

func isPKCEChallenge(c string) bool {
	if len(c) != pkceChallengeLen {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(c)
	return err == nil
}

func verifyPKCE(verifier, challenge string) bool {
	if verifier == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	got := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(got), []byte(challenge)) == 1
}

func isInstallID(id string) bool {
	if len(id) > maxInstallIDLen {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

func deviceName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return defaultDeviceName
	}
	if r := []rune(name); len(r) > maxDeviceNameLen {
		name = string(r[:maxDeviceNameLen])
	}
	return name
}
