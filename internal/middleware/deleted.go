package middleware

import (
	"context"
	"net/http"

	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"go.uber.org/zap"
)

// Deleted signs out the users whose account an admin deleted: their cookies
// are dropped (signOut) and the request goes on without their claims, so
// their database isn't created again. It runs after Auth.
func Deleted(isDeleted func(tenantID string) bool, signOut func(w http.ResponseWriter)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			tenantID := ""
			if claims, ok := ctx.Value(AccessClaimsCtxKey{}).(*jwt.AccessClaims); ok && claims != nil {
				tenantID = models.TenantID(claims.OAuthProvider, claims.OAuthID)
			} else if claims, ok := ctx.Value(RefreshClaimsCtxKey{}).(*jwt.RefreshClaims); ok && claims != nil {
				tenantID = models.TenantID(claims.OAuthProvider, claims.OAuthID)
			}
			if tenantID == "" || !isDeleted(tenantID) {
				next.ServeHTTP(w, r)
				return
			}

			logger.FromCtx(ctx).Info("signing out a deleted tenant", zap.String("tenantID", tenantID))
			signOut(w)
			ctx = context.WithValue(ctx, AccessClaimsCtxKey{}, nil)
			ctx = context.WithValue(ctx, RefreshClaimsCtxKey{}, nil)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
