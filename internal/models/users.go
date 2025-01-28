package models

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/omareloui/tyche/internal/idgen"
)

type OAuthProvider int

const (
	NilOAuthProvider OAuthProvider = iota
	Google
)

func (p OAuthProvider) String() string {
	return [...]string{"NilOAuthProvider", "Google"}[p]
}

type User struct {
	ID              string
	OAuthID         string        `validate:"required"`
	OAuthProvider   OAuthProvider `validate:"required"`
	Email           string        `validate:"required,email"`
	VerifiedEmail   bool
	FirstName       string    `validate:"required,min=2"`
	LastName        string    `validate:"required,min=2"`
	Picture         string    `validate:"required,url"`
	DefaultCurrency *Currency `validate:"required"`
	StartOfPeriod   uint8     `validate:"required,min=1,max=31"`
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
