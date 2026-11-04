package middleware

import (
	"context"
	"net/http"

	"github.com/omareloui/tyche/internal/models"
)

// BadgeRefresher counts the installed apps' badge again after a request of
// the tenant, write telling whether the request may have changed something.
type BadgeRefresher func(ctx context.Context, tenantID string, write bool)

// Badge refreshes the badge after the request (inside Vault / DeviceAuth, so
// the tenant's database can be opened with the request's key).
func Badge(refresh BadgeRefresher) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)

			claims := ClaimsFromCtx(r.Context())
			if claims == nil || claims.OAuthID == "" {
				return
			}
			write := r.Method != http.MethodGet && r.Method != http.MethodHead
			refresh(r.Context(), models.TenantID(claims.OAuthProvider, claims.OAuthID), write)
		})
	}
}
