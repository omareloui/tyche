package middleware

import (
	"context"
	"net/http"

	"github.com/omareloui/tyche/internal/models"
)

// DailyUpkeeper runs the tenant's daily upkeep: writing the recurrent
// transactions' occurrences due, purging the receipts scanned for a
// transaction never saved (once a day, so cheap otherwise).
type DailyUpkeeper func(ctx context.Context, tenantID string)

// DailyUpkeep runs the tenant's daily upkeep before the request (inside
// Vault / DeviceAuth, so the tenant's database can be opened with the
// request's key), for it to see the occurrences due and the badge counted
// after it to count them.
func DailyUpkeep(upkeep DailyUpkeeper) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if claims := ClaimsFromCtx(r.Context()); claims != nil && claims.OAuthID != "" {
				upkeep(r.Context(), models.TenantID(claims.OAuthProvider, claims.OAuthID))
			}
			next.ServeHTTP(w, r)
		})
	}
}
