package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
)

func TestAdmin(t *testing.T) {
	t.Setenv("ADMIN_TENANT_IDS", "GL_admin, GL_other")

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	admin := &jwt.AccessClaims{OAuthID: "admin", OAuthProvider: models.Google}
	user := &jwt.AccessClaims{OAuthID: "user", OAuthProvider: models.Google}

	tests := []struct {
		name   string
		method string
		claims *jwt.AccessClaims
		htmx   bool
		want   int
	}{
		{"no claims", http.MethodGet, nil, false, http.StatusNotFound},
		{"not admin", http.MethodGet, user, false, http.StatusNotFound},
		{"admin get", http.MethodGet, admin, false, http.StatusOK},
		{"admin post without htmx", http.MethodPost, admin, false, http.StatusNotFound},
		{"admin post", http.MethodPost, admin, true, http.StatusOK},
		{"not admin post", http.MethodPost, user, true, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, "/admin/fx-rates", nil)
			if tt.claims != nil {
				r = r.WithContext(context.WithValue(r.Context(), AccessClaimsCtxKey{}, tt.claims))
			}
			if tt.htmx {
				r.Header.Set("HX-Request", "true")
			}
			w := httptest.NewRecorder()
			Admin(ok).ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Errorf("got %d, want %d", w.Code, tt.want)
			}
		})
	}
}
