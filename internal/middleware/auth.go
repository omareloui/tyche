package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/omareloui/tyche/internal/errpage"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

type (
	AccessClaimsCtxKey  struct{}
	RefreshClaimsCtxKey struct{}
)

var RoutesToExcludeAuthMiddleware = []string{
	"/health", "/favicons", "/fonts", "/icons", "/js", "/styles",
}

// TokenRefresher issues new tokens for the user of the refresh claims in the
// request's context, setting their cookies, and returns the new access claims.
type TokenRefresher func(w http.ResponseWriter, r *http.Request) (*jwt.AccessClaims, error)

// Auth sets the access claims from the access token cookie. When it's expired
// and the refresh token is valid, the tokens are refreshed in place, so the
// request goes on with its method and body (a redirect would lose them).
func Auth(refresh TokenRefresher) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			currPath := r.URL.Path

			shouldIgnore := slicesutils.Contains(RoutesToExcludeAuthMiddleware, func(r string) bool {
				return strings.HasPrefix(currPath, r)
			})

			if shouldIgnore {
				next.ServeHTTP(w, r)
				return
			}

			var claims *jwt.AccessClaims

			accessCookie, err := r.Cookie("access_token")
			if !errors.Is(err, http.ErrNoCookie) {
				accessClaims, err := jwt.ParseAccessClaims(accessCookie.Value)
				if err == nil {
					claims = accessClaims
				}
			}

			if claims != nil {
				ctx := context.WithValue(r.Context(), AccessClaimsCtxKey{}, claims)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			refreshCookie, err := r.Cookie("refresh_token")
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			refreshClaims, err := jwt.ParseRefreshClaims(refreshCookie.Value)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			ctx := context.WithValue(r.Context(), RefreshClaimsCtxKey{}, refreshClaims)

			if strings.Contains(currPath, "oauth/refresh-tokens") {
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			claims, err = refresh(w, r.WithContext(ctx))
			if err != nil {
				logger.FromCtx(ctx).Warn("refreshing tokens", zap.Error(err))
				// Keeps the refresh claims, e.g. for the Vault middleware to
				// send the user to unlock their database when that's why.
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			ctx = context.WithValue(r.Context(), AccessClaimsCtxKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClaimsFromCtx are the access claims of the signed in user, nil without.
func ClaimsFromCtx(ctx context.Context) *jwt.AccessClaims {
	claims, _ := ctx.Value(AccessClaimsCtxKey{}).(*jwt.AccessClaims)
	return claims
}

// WriteError responds with the error page of code (the plain message for
// HTMX requests), with the request's claims and correlation ID.
func WriteError(w http.ResponseWriter, r *http.Request, code int, msg string) {
	errpage.Write(w, r, views.ErrorPageData{
		Claims: ClaimsFromCtx(r.Context()),
		Code:   code,
		Detail: msg,
		Ref:    CorrelationIDFromCtx(r.Context()),
	})
}

func Protected(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if accessClaims := r.Context().Value(AccessClaimsCtxKey{}); accessClaims == nil {
			logger.FromCtx(r.Context()).Error("trying to access protected route", zap.String("path", r.URL.Path))
			WriteError(w, r, http.StatusUnauthorized, "")
			return
		}

		next.ServeHTTP(w, r)
	})
}
