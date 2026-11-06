package jwt

import (
	"errors"
	"slices"
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
	// MonthStart is the day the user's months start on (models.MonthStart).
	MonthStart models.MonthStart
	// Timezone is the user's IANA time zone, "" in tokens issued before it
	// was added.
	Timezone string
	// Salary is the user's monthly salary, its amount 0 when not set (or in
	// tokens issued before it was added).
	Salary models.Salary
}

// Location is the user's time zone, the default one when unknown.
func (c *AccessClaims) Location() *time.Location {
	if c != nil {
		if loc, err := models.LoadTimezone(c.Timezone); err == nil {
			return loc
		}
	}
	loc, _ := models.LoadTimezone(models.DefaultTimezone)
	return loc
}

// Period is the user's month now is in.
func (c *AccessClaims) Period(now time.Time) models.Period {
	var s models.MonthStart
	if c != nil {
		s = c.MonthStart
	}
	return s.Of(now)
}

// IsAdmin reports whether the user is listed in ADMIN_TENANT_IDS.
func (c *AccessClaims) IsAdmin() bool {
	return c != nil && slices.Contains(config.GetAdminTenantIDs(), models.TenantID(c.OAuthProvider, c.OAuthID))
}

var (
	ErrInvalidTokenMethod = errors.New("invalid token method")
	ErrInvalidClaimsType  = errors.New("invalid claims type")
	ErrExpiredToken       = errors.New("expired token")
	ErrWrongTokenType     = errors.New("wrong token type")
)

const (
	typClaim        = "typ"
	deviceTokenType = "device"
)

type DeviceClaims struct {
	DeviceID string
	TokenID  string
	// VaultSecret unwraps the key of the user's encrypted database, "" when
	// it wasn't when the device signed in.
	VaultSecret   string
	UserID        string
	OAuthID       string
	OAuthProvider models.OAuthProvider
}

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
	if _, ok := (*claims)[typClaim]; ok {
		return nil, ErrWrongTokenType
	}

	var acc *models.Account
	var cur models.Currency

	if defaultAccountID, ok := (*claims)["default_account_id"]; ok {
		acc = &models.Account{
			ID:   defaultAccountID.(string),
			Name: (*claims)["default_account_name"].(string),
		}
	}

	if defaultCurrID, ok := (*claims)["default_currency_id"]; ok {
		cur.ID = defaultCurrID.(string)
		cur.String = (*claims)["default_currency_string"].(string)
		cur.Symbol = (*claims)["default_currency_symbol"].(string)
		cur.IbanPrefix = (*claims)["default_currency_iban_prefix"].(string)
		cur.Kind = models.CurrencyKind((*claims)["default_currency_kind"].(string))
		cur.IsShown = (*claims)["default_currency_is_shown"].(bool)
		cur.IsDefault = true
	}

	monthStart, _ := (*claims)["month_start"].(float64)
	timezone, _ := (*claims)["timezone"].(string)
	salary, _ := (*claims)["salary"].(float64)
	salaryCur, _ := (*claims)["salary_currency"].(string)

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
		// Tokens issued before it was added have none: 1st of the month.
		MonthStart: models.MonthStart(monthStart),
		Timezone:   timezone,
		Salary:     models.Salary{Amount: salary, Currency: salaryCur},
	}, nil
}

func ParseRefreshClaims(token string) (*RefreshClaims, error) {
	claims, err := parse(token)
	if err != nil {
		return nil, err
	}
	if _, ok := (*claims)[typClaim]; ok {
		return nil, ErrWrongTokenType
	}
	return &RefreshClaims{
		ID:            (*claims)["id"].(string),
		OAuthID:       (*claims)["oauth_id"].(string),
		OAuthProvider: models.OAuthProvider((*claims)["oauth_provider"].(float64)),
	}, nil
}

// NewDeviceToken creates a non-expiring token for a mobile device. Revoking is
// done by revoking the device in the user's database, or replacing its token
// id there. vaultSecret is the secret of the device's vault slot when the
// user's database is encrypted, else "".
func NewDeviceToken(usr *models.User, deviceID, tokenID, vaultSecret string) (string, error) {
	claims := jwt.MapClaims{
		typClaim:         deviceTokenType,
		"device_id":      deviceID,
		"token_id":       tokenID,
		"id":             usr.ID,
		"oauth_id":       usr.OAuthID,
		"oauth_provider": uint64(usr.OAuthProvider),
		"iat":            time.Now().Unix(),
	}
	if vaultSecret != "" {
		claims["vault_secret"] = vaultSecret
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(config.GetJWTSecret())
}

func ParseDeviceClaims(token string) (*DeviceClaims, error) {
	claims, err := parse(token)
	if err != nil {
		return nil, err
	}
	if typ, _ := (*claims)[typClaim].(string); typ != deviceTokenType {
		return nil, ErrWrongTokenType
	}

	deviceID, _ := (*claims)["device_id"].(string)
	tokenID, _ := (*claims)["token_id"].(string)
	vaultSecret, _ := (*claims)["vault_secret"].(string)
	userID, _ := (*claims)["id"].(string)
	oauthID, _ := (*claims)["oauth_id"].(string)
	oauthProvider, ok := (*claims)["oauth_provider"].(float64)
	if deviceID == "" || oauthID == "" || !ok {
		return nil, ErrInvalidClaimsType
	}

	return &DeviceClaims{
		DeviceID:      deviceID,
		TokenID:       tokenID,
		VaultSecret:   vaultSecret,
		UserID:        userID,
		OAuthID:       oauthID,
		OAuthProvider: models.OAuthProvider(oauthProvider),
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
			"picture":    usr.AvatarURL(),

			"default_currency_id":          usr.DefaultCurrency.ID,
			"default_currency_string":      usr.DefaultCurrency.String,
			"default_currency_symbol":      usr.DefaultCurrency.Symbol,
			"default_currency_kind":        usr.DefaultCurrency.Kind,
			"default_currency_iban_prefix": usr.DefaultCurrency.IbanPrefix,
			"default_currency_is_shown":    usr.DefaultCurrency.IsShown,

			"month_start": usr.StartOfPeriod,
			"timezone":    usr.Timezone,

			"iat": now.Unix(),
			"exp": exp.Unix(),
		}

		if usr.DefaultAccount != nil {
			(*claims)["default_account_id"] = usr.DefaultAccount.ID
			(*claims)["default_account_name"] = usr.DefaultAccount.Name
		}
		if usr.MonthlySalary > 0 {
			(*claims)["salary"] = usr.MonthlySalary
			(*claims)["salary_currency"] = usr.SalaryCurrency
		}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenStr, err := token.SignedString(config.GetJWTSecret())
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenStr, exp, nil
}
