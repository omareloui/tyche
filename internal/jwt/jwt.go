package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/models"
)

type TokenType int

const (
	accessToken TokenType = iota
	refreshToken

	refreshExpiration = time.Hour * 24 * 30
	accessExpiration  = time.Minute * 15
)

type tokenDetails struct {
	Encoded    string
	Expiration time.Time
}

type TokenPair struct {
	Refresh tokenDetails
	Access  tokenDetails
}

type ClaimsPair struct {
	Refresh RefreshClaims
	Access  AccessClaims
}

type RefreshClaims struct {
	ID            string
	OAuthID       string
	OAuthProvider models.OAuthProvider
}

type AccessClaims struct {
	ID              string
	OAuthID         string
	OAuthProvider   models.OAuthProvider
	Email           string
	Name            string
	FirstName       string
	LastName        string
	Picture         string
	DefaultCurrency models.Currency
	DefaultAccount  *models.Account
}

var (
	ErrInvalidTokenMethod = errors.New("invalid token method")
	ErrInvalidClaimsType  = errors.New("invalid claims type")
	ErrExpiredToken       = errors.New("expired token")
)

func NewPair(usr *models.User) (*TokenPair, error) {
	refresh, refreshExp, err := newToken(refreshToken, usr)
	if err != nil {
		return nil, err
	}
	access, accessExp, err := newToken(accessToken, usr)
	if err != nil {
		return nil, err
	}
	return &TokenPair{Access: tokenDetails{access, accessExp}, Refresh: tokenDetails{refresh, refreshExp}}, nil
}

func ParseAccessClaims(token string) (*AccessClaims, error) {
	claims, err := parse(token)
	if err != nil {
		return nil, err
	}

	var acc *models.Account
	var cur models.Currency

	if defaultAccountId, ok := (*claims)["default_account_id"]; ok {
		acc = &models.Account{
			ID:   defaultAccountId.(string),
			Name: (*claims)["default_account_name"].(string),
		}
	}

	if defaultCurrId, ok := (*claims)["default_currency_id"]; ok {
		cur.ID = defaultCurrId.(string)
		cur.String = (*claims)["default_currency_string"].(string)
		cur.Symbol = (*claims)["default_currency_symbol"].(string)
		cur.IbanPrefix = (*claims)["default_currency_iban_prefix"].(string)
		cur.Kind = models.CurrencyKind((*claims)["default_currency_kind"].(string))
		cur.IsShown = (*claims)["default_currency_is_shown"].(bool)
		cur.IsDefault = true
	}

	return &AccessClaims{
		ID:              (*claims)["id"].(string),
		OAuthID:         (*claims)["oauth_id"].(string),
		OAuthProvider:   models.OAuthProvider((*claims)["oauth_provider"].(float64)),
		Name:            (*claims)["name"].(string),
		Email:           (*claims)["email"].(string),
		FirstName:       (*claims)["first_name"].(string),
		LastName:        (*claims)["last_name"].(string),
		Picture:         (*claims)["picture"].(string),
		DefaultCurrency: cur,
		DefaultAccount:  acc,
	}, nil
}

func ParseRefreshClaims(token string) (*RefreshClaims, error) {
	claims, err := parse(token)
	if err != nil {
		return nil, err
	}
	return &RefreshClaims{
		ID:            (*claims)["id"].(string),
		OAuthID:       (*claims)["oauth_id"].(string),
		OAuthProvider: models.OAuthProvider((*claims)["oauth_provider"].(float64)),
	}, nil
}

func parse(tokenStr string) (*jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidTokenMethod
		}
		return config.GetJWTSecret(), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidClaimsType
	}

	return &claims, nil
}

func newToken(kind TokenType, usr *models.User) (string, time.Time, error) {
	var exp time.Time
	var claims *jwt.MapClaims

	now := time.Now()

	if kind == refreshToken {
		exp = time.Now().Add(refreshExpiration)
		claims = &jwt.MapClaims{
			"id": usr.ID,

			"oauth_id":       usr.OAuthID,
			"oauth_provider": uint64(usr.OAuthProvider),

			"iat": now.Unix(),
			"exp": exp.Unix(),
		}
	} else {
		exp = time.Now().Add(accessExpiration)
		claims = &jwt.MapClaims{
			"id": usr.ID,

			"oauth_id":       usr.OAuthID,
			"oauth_provider": uint64(usr.OAuthProvider),

			"name":       usr.FullName(),
			"email":      usr.Email,
			"first_name": usr.FirstName,
			"last_name":  usr.LastName,
			"picture":    usr.Picture,

			"default_currency_id":          usr.DefaultCurrency.ID,
			"default_currency_string":      usr.DefaultCurrency.String,
			"default_currency_symbol":      usr.DefaultCurrency.Symbol,
			"default_currency_kind":        usr.DefaultCurrency.Kind,
			"default_currency_iban_prefix": usr.DefaultCurrency.IbanPrefix,
			"default_currency_is_shown":    usr.DefaultCurrency.IsShown,

			"iat": now.Unix(),
			"exp": exp.Unix(),
		}

		if usr.DefaultAccount != nil {
			(*claims)["default_account_id"] = usr.DefaultAccount.ID
			(*claims)["default_account_name"] = usr.DefaultAccount.Name
		}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenStr, err := token.SignedString(config.GetJWTSecret())
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenStr, exp, nil
}
