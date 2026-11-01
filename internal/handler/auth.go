package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	accessTokenCookieName  = "access_token"
	refreshTokenCookieName = "refresh_token"
)

var oauthConf *oauth2.Config

func (h *Handler) OAuth(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())
	csrfId := idgen.New()
	oauthStates.add(csrfId, nil)
	url := getOAuth2Conf().AuthCodeURL(csrfId)
	l.Debug("going to sign a user", zap.String("url", url), zap.String("csrfId", csrfId))
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	return nil, nil
}

func (h *Handler) OAuthCB(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	state := r.FormValue("state")
	mobile, ok := oauthStates.pop(state)
	if !ok {
		err := fmt.Errorf("invalid oauth state, got '%s'", state)
		l.Error("preventing CSRF", zap.Error(err))
		return nil, err
	}

	gUsr, err := h.googleUser(r.Context(), r.FormValue("code"))
	if err != nil {
		return nil, err
	}

	ctx := context.WithValue(r.Context(), middleware.AccessClaimsCtxKey{}, &jwt.AccessClaims{
		OAuthID:       gUsr.OAuthID,
		OAuthProvider: models.Google,
	})

	// An encrypted database: the user is in it, so it's unlocked first,
	// from the browser's session or else on the unlock page.
	tenantID := models.TenantID(models.Google, gUsr.OAuthID)
	key, encrypted, err := h.vaultKey(r, tenantID)
	if err != nil {
		return nil, err
	}
	if encrypted && key == nil {
		l.Info("signing in to a locked database", zap.String("tenantID", tenantID))
		return nil, h.startPendingUnlock(w, r, pendingUnlock{provider: models.Google, oauthID: gUsr.OAuthID, mobile: mobile})
	}
	if key != nil {
		ctx = context.WithValue(ctx, middleware.VaultKeyCtxKey{}, key)
	}

	usr, err := h.signInUser(ctx, gUsr)
	if err != nil {
		return nil, err
	}

	if mobile != nil {
		code, err := mobileCodes.add(usr, mobile, key)
		if err != nil {
			l.Error("generating mobile code", zap.Error(err))
			return nil, err
		}
		l.Info("signed user from mobile", zap.String("oauth_id", usr.OAuthID))
		http.Redirect(w, r, mobileRedirectURI+"?code="+url.QueryEscape(code), http.StatusTemporaryRedirect)
		return nil, nil
	}

	l.Debug("generating cookies for the user", zap.Any("user", usr))

	if _, err := setJWTCookies(w, usr); err != nil {
		l.Error("error generating jwt cookies", zap.Error(err), zap.Any("user", usr))
		return nil, err
	}

	l.Info("new user singed", zap.Any("user", usr))

	// Asked once whether to encrypt their database.
	next := "/"
	if !usr.EncryptionPrompted && !encrypted {
		next = "/security?welcome"
	}
	http.Redirect(w, r, next, http.StatusTemporaryRedirect)
	return nil, nil
}

// googleUser exchanges the oauth code and gets the user's google info.
func (h *Handler) googleUser(ctx context.Context, code string) (*models.User, error) {
	l := logger.FromCtx(ctx)
	conf := getOAuth2Conf()

	token, err := conf.Exchange(ctx, code)
	if err != nil {
		l.Error("code exchange failed", zap.Error(err))
		return nil, err
	}

	client := conf.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		l.Error("getting user info", zap.Error(err))
		return nil, err
	}
	defer resp.Body.Close()

	_usr, err := models.UserFromGoogleUserBody(resp.Body)
	if err != nil {
		l.Error("parsing json user", zap.Error(err))
		return nil, err
	}
	l.Debug("user info", zap.Any("user", _usr))
	return _usr, nil
}

// signInUser gets the user of the google info, creating them if it's their
// first sign in. The context has their (access) claims.
func (h *Handler) signInUser(ctx context.Context, _usr *models.User) (*models.User, error) {
	l := logger.FromCtx(ctx)

	usr, err := h.repo.GetUserByOAuthID(ctx, _usr.OAuthID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}

		l.Debug("didn't find the user, creating a new one")

		if err = h.initUser(ctx, _usr); err != nil {
			l.Error("error initing user", zap.Error(err))
			return nil, err
		}
		usr = _usr
	}

	return usr, nil
}

func (h *Handler) RefreshTokens(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	if _, err := h.RefreshSession(w, r); err != nil {
		if errors.Is(err, jwt.ErrExpiredToken) {
			return nil, err
		}
		if errors.Is(err, repository.ErrTenantLocked) {
			http.Redirect(w, r, middleware.UnlockURL(safeNext(r.URL.Query().Get("next"))), http.StatusSeeOther)
			return nil, nil
		}
		logger.FromCtx(r.Context()).Info("signing out.")
		return h.Signout(w, r)
	}

	next := r.URL.Query().Get("next")
	if next == "" {
		next = "/"
	}

	http.Redirect(w, r, next, http.StatusTemporaryRedirect)

	return nil, nil
}

// RefreshSession is the middleware.TokenRefresher: it issues new tokens for the
// user of the refresh claims in the context. When the user is gone, the
// cookies are unset.
func (h *Handler) RefreshSession(w http.ResponseWriter, r *http.Request) (*jwt.AccessClaims, error) {
	l := logger.FromCtx(r.Context())

	claims, ok := r.Context().Value(middleware.RefreshClaimsCtxKey{}).(*jwt.RefreshClaims)
	if !ok {
		l.Error("can't find the refresh token to refresh the tokens")
		return nil, jwt.ErrExpiredToken
	}

	l.Debug("refreshing tokens for user", zap.String("id", claims.ID), zap.String("oauth_id", claims.OAuthID))

	// An encrypted database is read with the browser's session, the Vault
	// middleware sends the user to unlock it when it's locked.
	ctx, err := h.withVaultKey(r.Context(), r, models.TenantID(claims.OAuthProvider, claims.OAuthID))
	if err != nil {
		return nil, err
	}

	usr, err := h.repo.GetUserByOAuthID(ctx, claims.OAuthID)
	if err != nil || usr == nil {
		l.Error("refreshing token. can't get the user to refresh the token.", zap.String("oauth_id", claims.OAuthID), zap.Error(err))
		unsetCookie(w, refreshTokenCookieName)
		unsetCookie(w, accessTokenCookieName)
		return nil, fmt.Errorf("getting the user to refresh the tokens: %w", err)
	}

	tokens, err := setJWTCookies(w, usr)
	if err != nil {
		l.Error("error generating jwt cookies", zap.Error(err), zap.Any("user", usr))
		return nil, err
	}

	return jwt.ParseAccessClaims(tokens.Access.Encoded)
}

func (h *Handler) Signout(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	h.endVaultSession(r.Context(), w, r)
	unsetCookie(w, refreshTokenCookieName)
	unsetCookie(w, accessTokenCookieName)
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	return nil, nil
}

func (h *Handler) initUser(ctx context.Context, usr *models.User) error {
	cur, err := h.repo.GetCurrencyByString(ctx, models.GetDefaultCurrency().String)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			l := logger.FromCtx(ctx)
			l.Error("couldn't find currencies to create a user, try seeding the database")
		}
		return err
	}
	usr.DefaultCurrency = *cur

	if err := h.repo.CreateUser(ctx, usr); err != nil {
		return err
	}

	return nil
}

func getOAuth2Conf() *oauth2.Config {
	if oauthConf == nil {
		oauthConf = &oauth2.Config{
			ClientID:     config.GetGoogleOAuthClientID(),
			ClientSecret: config.GetGoogleOAuthClientSecret(),
			RedirectURL:  config.GetOAuthCBEndpoint(),
			Scopes: []string{
				"https://www.googleapis.com/auth/userinfo.email",
				"https://www.googleapis.com/auth/userinfo.profile",
			},
			Endpoint: google.Endpoint,
		}
	}

	return oauthConf
}

// setJWTCookies issues a new token pair for the user and sets its cookies.
func setJWTCookies(w http.ResponseWriter, usr *models.User) (*jwt.TokenPair, error) {
	tokens, err := jwt.NewPair(usr)
	if err != nil {
		return nil, err
	}

	http.SetCookie(w, newCookie(accessTokenCookieName, tokens.Access.Encoded, tokens.Access.Expiration))
	http.SetCookie(w, newCookie(refreshTokenCookieName, tokens.Refresh.Encoded, tokens.Refresh.Expiration))

	return tokens, nil
}

func newCookie(name, value string, exp time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		HttpOnly: true,
		Expires:  exp,
		Path:     "/",
	}
}

func unsetCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		HttpOnly: true,
		Expires:  time.Unix(0, 0),
		Path:     "/",
	})
}
