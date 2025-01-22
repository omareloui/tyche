package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/logger"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	oauthConf *oauth2.Config
	// TODO: In production, generate this randomly for each request
	oauthStateString = "random-string"
)

type GoogleUser struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
}

func (h *Handler) OAuth(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	url := getOAuth2Conf().AuthCodeURL(oauthStateString)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	return nil, nil
}

func (h *Handler) OAuthCB(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())
	conf := getOAuth2Conf()

	state := r.FormValue("state")
	if state != oauthStateString {
		err := fmt.Errorf("invalid oauth state, expected '%s', got '%s'", oauthStateString, state)
		l.Error("preventing CSRF", zap.Error(err))
		return nil, err
	}

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

	var googleUser GoogleUser
	if err := json.NewDecoder(resp.Body).Decode(&googleUser); err != nil {
		l.Error("parsing json user", zap.Error(err))
		return nil, err
	}

	// At this point, you have the user's information in the googleUser struct
	// You can:
	// 1. Create a session
	// 2. Create or update user in your database
	// 3. Generate JWT token
	// 4. Redirect to frontend with token/session

	fmt.Printf("User: %+v\n", googleUser)

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
