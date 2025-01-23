package middleware

import (
	"context"
	"net/http"

	"github.com/omareloui/tyche/internal/jwt"
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
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
