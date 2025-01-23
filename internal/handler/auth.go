package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/logger"
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
	csrfId := idgen.New()
	oauthStateStrings[csrfId] = struct{}{}
	url := getOAuth2Conf().AuthCodeURL(csrfId)
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

	var usr *models.User

	if err := json.NewDecoder(resp.Body).Decode(&usr); err != nil {
		l.Error("parsing json user", zap.Error(err))
		return nil, err
	}

	myGoogleID := "110711621731459269840"
	if usr.OAuthID != myGoogleID {
		err := errors.New("the site isn't available to the public yet")
		l.Error("invalid user", zap.Error(err), zap.Any("user", usr))
		return nil, err
	}

	// TODO: safe the user in my db and connect it with his/her own db

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
