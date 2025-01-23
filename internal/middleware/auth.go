package middleware

import (
	"context"
	"net/http"

	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/resperr"
	"go.uber.org/zap"
)

type AccessClaimsCtxKey struct{}

func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("access_token")
		if err != nil || cookie == nil {
			next.ServeHTTP(w, r)
			return
		}

		accessClaims, err := jwt.ParseAccessClaims(cookie.Value)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), AccessClaimsCtxKey{}, accessClaims)
		next.ServeHTTP(w, r.WithContext(ctx))
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
