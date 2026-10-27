package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/logger"
	"go.uber.org/zap"
)

type DeviceIDCtxKey struct{}

// DeviceVerifier checks the device is still allowed, the context already has
// the device owner's access claims.
type DeviceVerifier func(ctx context.Context, deviceID string) error

// DeviceAuth authenticates mobile devices using the bearer device token, then
// sets the access claims so the tenant database can be resolved.
func DeviceAuth(verify DeviceVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			l := logger.FromCtx(r.Context())

			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || token == "" {
				jsonUnauthorized(w)
				return
			}

			claims, err := jwt.ParseDeviceClaims(token)
			if err != nil {
				l.Warn("invalid device token", zap.Error(err))
				jsonUnauthorized(w)
				return
			}

			ctx := context.WithValue(r.Context(), AccessClaimsCtxKey{}, &jwt.AccessClaims{
				ID:            claims.UserID,
				OAuthID:       claims.OAuthID,
				OAuthProvider: claims.OAuthProvider,
			})
			ctx = context.WithValue(ctx, DeviceIDCtxKey{}, claims.DeviceID)

			if err := verify(ctx, claims.DeviceID); err != nil {
				l.Warn("device not allowed", zap.String("device_id", claims.DeviceID), zap.Error(err))
				jsonUnauthorized(w)
				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func jsonUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
}
