package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/slicesutils"
	"go.uber.org/zap"
)

type (
	AccessClaimsCtxKey  struct{}
	RefreshClaimsCtxKey struct{}
)

var RoutesToExcludeAuthMiddleware = []string{
	"/health", "/favicons", "/fonts", "/icons", "/js", "/styles",
}

func Auth(next http.Handler) http.Handler {
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

		isRefreshTokenRoute := strings.Contains(currPath, "oauth/refresh-tokens")
		refreshCookie, err := r.Cookie("refresh_token")
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		if !isRefreshTokenRoute {
			queries := r.URL.Query()
			next := queries.Get("next")
			if next == "" {
				next = currPath
			}
			path := fmt.Sprintf("/oauth/refresh-tokens?next=%s", next)
			http.Redirect(w, r, path, http.StatusTemporaryRedirect)
			return
		} else {
			refreshClaims, err := jwt.ParseRefreshClaims(refreshCookie.Value)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			ctx := context.WithValue(r.Context(), RefreshClaimsCtxKey{}, refreshClaims)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
	})
}

func Protected(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if accessClaims := r.Context().Value(AccessClaimsCtxKey{}); accessClaims == nil {
			l := logger.FromCtx(r.Context())

			httperr := resperr.New(http.StatusUnauthorized, "")
			w.Header().Set("Content-Type", "text/html")
			l.Error("trying to access protected route", zap.String("path", r.URL.Path))
			http.Error(w, httperr.Message, httperr.Code)
			return
		}

		next.ServeHTTP(w, r)
	})
}
