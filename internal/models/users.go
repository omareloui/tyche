package models

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/omareloui/tyche/internal/idgen"
)

type OAuthProvider uint8

const (
	NilOAuthProvider OAuthProvider = iota
	Google
	Facebook
	Microsoft
	Github
)

func (p OAuthProvider) String() string {
	return [...]string{
		"NilOAuthProvider", "Google",
		"Facebook", "Microsofto",
		"Github",
	}[p]
}

// TenantID identifies a user across the app, and names their database file.
func TenantID(p OAuthProvider, oauthID string) string {
	return fmt.Sprintf("%s_%s", p.Prefix(), oauthID)
}

func (p OAuthProvider) Prefix() string {
	return [...]string{
		"??", "GL",
		"FB", "MS",
		"GH",
	}[p]
}

type User struct {
	ID            string
	OAuthID       string        `validate:"required"`
	OAuthProvider OAuthProvider `validate:"required"`
	Email         string        `validate:"required,email"`
	VerifiedEmail bool
	FirstName     string `validate:"required,min=2"`
	LastName      string `validate:"required,min=2"`
	Picture       string `validate:"required,url"`
	// PictureHash versions the downloaded picture served at /avatar, "" when
	// it wasn't downloaded.
	PictureHash     string
	DefaultCurrency Currency `validate:"required"`
	DefaultAccount  *Account `validate:"required"`
	StartOfPeriod   uint8    `validate:"required,min=1,max=31"`
	// Timezone is the user's IANA time zone (e.g. Africa/Cairo).
	Timezone string
	SeededAt        time.Time
	// Onboarded is whether the user finished (or skipped) the onboarding at
	// /welcome, shown on signing in until then.
	Onboarded bool
	CreatedAt time.Time
}

// AvatarURL is the url of the user's picture: the downloaded one, else the
// provider's.
func (u *User) AvatarURL() string {
	if u.PictureHash != "" {
		return "/avatar?v=" + u.PictureHash
	}
	return u.Picture
}

func (u *User) FullName() string {
	return fmt.Sprintf("%s %s", u.FirstName, u.LastName)
}

type googleUser struct {
	OAuthID       string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	FirstName     string `json:"given_name"`
	LastName      string `json:"family_name"`
	Picture       string `json:"picture"`
}

func UserFromGoogleUserBody(googleU io.Reader) (*User, error) {
	var usr googleUser

	if err := json.NewDecoder(googleU).Decode(&usr); err != nil {
		return nil, err
	}

	return &User{
		ID:            idgen.New(),
		OAuthID:       usr.OAuthID,
		OAuthProvider: Google,
		Email:         usr.Email,
		VerifiedEmail: usr.VerifiedEmail,
		FirstName:     usr.FirstName,
		LastName:      usr.LastName,
		Picture:       usr.Picture,
	}, nil
}
