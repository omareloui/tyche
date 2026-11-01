package middleware

import (
	"net/http"

	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/logger"
	"go.uber.org/zap"
)

// Admin lets only the users in ADMIN_TENANT_IDS through, signed in from the
// web (the access token cookie). Everyone else gets a 404, so the routes look
// like they don't exist. Unsafe methods also need the HX-Request header, which
// a cross-site form can't send.
func Admin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, _ := r.Context().Value(AccessClaimsCtxKey{}).(*jwt.AccessClaims)

		isSafe := r.Method == http.MethodGet || r.Method == http.MethodHead
		if !claims.IsAdmin() || (!isSafe && r.Header.Get("HX-Request") != "true") {
			logger.FromCtx(r.Context()).Warn("non admin trying to access an admin route", zap.String("path", r.URL.Path))
			WriteError(w, r, http.StatusNotFound, "")
			return
		}

		next.ServeHTTP(w, r)
	})
}
