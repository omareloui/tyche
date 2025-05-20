package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/models"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	accessTokenCookieName  = "access_token"
	refreshTokenCookieName = "refresh_token"
)

var (
	oauthConf         *oauth2.Config
	oauthStateStrings = make(map[string]struct{})
)

func (h *Handler) OAuth(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())
	csrfId := idgen.New()
	oauthStateStrings[csrfId] = struct{}{}
	url := getOAuth2Conf().AuthCodeURL(csrfId)
	l.Debug("going to sign a user", zap.String("url", url), zap.String("csrfId", csrfId))
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	return nil, nil
}

func (h *Handler) OAuthCB(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())
	conf := getOAuth2Conf()

	state := r.FormValue("state")
	if _, ok := oauthStateStrings[state]; !ok {
		err := fmt.Errorf("invalid oauth state, got '%s'", state)
		l.Error("preventing CSRF", zap.Error(err))
		return nil, err
	}
	delete(oauthStateStrings, state)

	code := r.FormValue("code")
	token, err := conf.Exchange(r.Context(), code)
	if err != nil {
		l.Error("code exchange failed", zap.Error(err))
		return nil, err
	}

	client := conf.Client(r.Context(), token)
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

	accessCtx := context.WithValue(r.Context(), middleware.AccessClaimsCtxKey{}, &jwt.AccessClaims{
		OAuthID:       _usr.OAuthID,
		OAuthProvider: models.Google,
	})

	usr, err := h.repo.GetUserByOAuthID(accessCtx, _usr.OAuthID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}

		l.Debug("didn't find the user, creating a new one")

		if err = h.initUser(accessCtx, _usr); err != nil {
			l.Error("error initing user", zap.Error(err))
			return nil, err
		}
		usr = _usr
	}

	l.Debug("generating cookies for the user", zap.Any("user", _usr))

	access, refresh, err := generateJWTCookies(usr)
	if err != nil {
		l.Error("error generating jwt cookies", zap.Error(err), zap.Any("user", usr))
		return nil, err
	}

	http.SetCookie(w, access)
	http.SetCookie(w, refresh)

	l.Info("new user singed", zap.Any("user", usr))

	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	return nil, nil
}

func (h *Handler) Signout(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	unsetCookie(w, refreshTokenCookieName)
	unsetCookie(w, accessTokenCookieName)
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	return nil, nil
}

func (h *Handler) initUser(ctx context.Context, usr *models.User) error {
	cur, err := h.repo.GetCurrencyByEnum(ctx, models.CurrencyUsd)
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

func generateJWTCookies(usr *models.User) (access *http.Cookie, refresh *http.Cookie, err error) {
	var tokens *jwt.TokenPair

	tokens, err = jwt.NewPair(usr)
	if err != nil {
		return nil, nil, err
	}

	access = newCookie(refreshTokenCookieName, tokens.Refresh.Encoded, tokens.Refresh.Expiration)
	refresh = newCookie(accessTokenCookieName, tokens.Access.Encoded, tokens.Access.Expiration)

	return access, refresh, nil
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
