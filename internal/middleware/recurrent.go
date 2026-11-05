package middleware

import (
	"context"
	"net/http"

	"github.com/omareloui/tyche/internal/models"
)

// RecurrentTopUpper writes the recurrent transactions' occurrences due of the
// tenant (once a day, so cheap otherwise).
type RecurrentTopUpper func(ctx context.Context, tenantID string)

// RecurrentTopUp writes the recurrent transactions' occurrences due before
// the request (inside Vault / DeviceAuth, so the tenant's database can be
// opened with the request's key), for it to see them and the badge counted
// after it to count them.
func RecurrentTopUp(topUp RecurrentTopUpper) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if claims := ClaimsFromCtx(r.Context()); claims != nil && claims.OAuthID != "" {
				topUp(r.Context(), models.TenantID(claims.OAuthProvider, claims.OAuthID))
			}
			next.ServeHTTP(w, r)
		})
	}
}
