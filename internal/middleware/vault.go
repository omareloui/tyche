package middleware

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/omareloui/tyche/internal/errpage"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

// VaultKeyCtxKey holds the key of the tenant's encrypted database.
type VaultKeyCtxKey struct{}

// VaultOpener gets the key of the request's tenant database from its unlocked
// session. encrypted is false when the database isn't encrypted (key is nil
// then), key is nil when it is but the request has no (valid) session.
type VaultOpener func(r *http.Request) (key []byte, encrypted bool, err error)

// UnlockPath is the page unlocking an encrypted database.
const UnlockPath = "/unlock"

// routesWithoutVault work with a locked database (they must not read it).
var routesWithoutVault = []string{UnlockPath, "/oauth", "/signout"}

// Vault puts the key of the user's encrypted database in the context, from
// their unlocked session. Without one, they're sent to unlock it. It runs
// after Auth, as it's the claims that tell whose database it is.
func Vault(open VaultOpener) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if ctx.Value(AccessClaimsCtxKey{}) == nil && ctx.Value(RefreshClaimsCtxKey{}) == nil {
				next.ServeHTTP(w, r)
				return
			}

			key, encrypted, err := open(r)
			if err != nil {
				logger.FromCtx(ctx).Error("opening the vault", zap.Error(err))
				WriteError(w, r, http.StatusInternalServerError, "")
				return
			}
			if !encrypted {
				next.ServeHTTP(w, r)
				return
			}
			if key != nil {
				next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, VaultKeyCtxKey{}, key)))
				return
			}

			if slicesutils.Contains(routesWithoutVault, func(p string) bool { return strings.HasPrefix(r.URL.Path, p) }) {
				next.ServeHTTP(w, r)
				return
			}
			RedirectToUnlock(w, r)
		})
	}
}

// RedirectToUnlock sends the user to unlock their database, coming back to
// the page they were on.
func RedirectToUnlock(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		back := "/"
		if u, err := url.Parse(r.Header.Get("HX-Current-URL")); err == nil && u.Path != "" {
			back = u.RequestURI()
		}
		w.Header().Set("HX-Redirect", UnlockURL(back))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodGet {
		errpage.Write(w, r, views.ErrorPageData{
			Claims:    ClaimsFromCtx(r.Context()),
			Code:      http.StatusUnauthorized,
			Ref:       CorrelationIDFromCtx(r.Context()),
			UnlockURL: UnlockPath,
		})
		return
	}
	http.Redirect(w, r, UnlockURL(r.URL.RequestURI()), http.StatusSeeOther)
}

// UnlockURL is the unlock page, then going to next.
func UnlockURL(next string) string {
	if next == "" || next == "/" {
		return UnlockPath
	}
	return UnlockPath + "?next=" + url.QueryEscape(next)
}

// HTMXOnly rejects unsafe requests without the HX-Request header, which a
// cross-site form can't send (e.g. encrypting someone's database with a
// password of the attacker's).
func HTMXOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isSafe := r.Method == http.MethodGet || r.Method == http.MethodHead
		if !isSafe && r.Header.Get("HX-Request") != "true" {
			WriteError(w, r, http.StatusForbidden, "")
			return
		}
		next.ServeHTTP(w, r)
	})
}
