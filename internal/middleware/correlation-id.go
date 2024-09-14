package middleware

import (
	"context"
	"net/http"

	"github.com/omareloui/financial-planner/internal/idgen"
)

type correlationIDCtxKey struct{}

func CorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := idgen.New()
		ctx := context.WithValue(r.Context(), correlationIDCtxKey{}, correlationID)
		w.Header().Add("X-Correlation-ID", correlationID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
