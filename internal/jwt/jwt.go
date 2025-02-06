package jwt

import (
	"errors"
	"strconv"
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
	// TODO: update on prod
	accessExpiration = time.Hour * 24 * 30
	// accessExpiration  = time.Minute * 15
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
	ID string
}

type AccessClaims struct {
	ID            string
	OAuthID       string
	OAuthProvider models.OAuthProvider
	Email         string
	Name          string
	FirstName     string
	LastName      string
	Picture       string
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
	return &AccessClaims{
		ID:            (*claims)["id"].(string),
		OAuthID:       (*claims)["oauth_id"].(string),
		OAuthProvider: (*claims)["oauth_provider"].(models.OAuthProvider),
		Name:          (*claims)["name"].(string),
		Email:         (*claims)["email"].(string),
		FirstName:     (*claims)["first_name"].(string),
		LastName:      (*claims)["last_name"].(string),
		Picture:       (*claims)["picture"].(string),
	}, nil
}

func ParseRefreshClaims(token string) (*RefreshClaims, error) {
	claims, err := parse(token)
	if err != nil {
		return nil, err
	}
	return &RefreshClaims{ID: (*claims)["id"].(string)}, nil
}

func parse(tokenStr string) (*jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
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

			"iat": now.Unix(),
			"exp": exp.Unix(),
		}
	} else {
		exp = time.Now().Add(accessExpiration)
		claims = &jwt.MapClaims{
			"id": usr.ID,

			"oauth_id":       usr.OAuthID,
			"oauth_provider": strconv.FormatUint(uint64(usr.OAuthProvider), 10),

			"name":       usr.FullName(),
			"email":      usr.Email,
			"first_name": usr.FirstName,
			"last_name":  usr.LastName,
			"picture":    usr.Picture,

			"iat": now.Unix(),
			"exp": exp.Unix(),
		}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenStr, err := token.SignedString(config.GetJWTSecret())
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenStr, exp, nil
}
