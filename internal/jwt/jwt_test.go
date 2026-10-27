package jwt_test

import (
	"errors"
	"testing"

	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
)

func TestDeviceToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")

	usr := &models.User{ID: "user", OAuthID: "123", OAuthProvider: models.Google}

	token, err := jwt.NewDeviceToken(usr, "device")
	if err != nil {
		t.Fatal(err)
	}

	claims, err := jwt.ParseDeviceClaims(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.DeviceID != "device" || claims.OAuthID != "123" || claims.OAuthProvider != models.Google || claims.UserID != "user" {
		t.Errorf("unexpected claims %+v", claims)
	}

	if _, err := jwt.ParseAccessClaims(token); !errors.Is(err, jwt.ErrWrongTokenType) {
		t.Errorf("device token accepted as access token, err: %v", err)
	}
	if _, err := jwt.ParseRefreshClaims(token); !errors.Is(err, jwt.ErrWrongTokenType) {
		t.Errorf("device token accepted as refresh token, err: %v", err)
	}

	pair, err := jwt.NewPair(usr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jwt.ParseDeviceClaims(pair.Access.Encoded); !errors.Is(err, jwt.ErrWrongTokenType) {
		t.Errorf("access token accepted as device token, err: %v", err)
	}
}
