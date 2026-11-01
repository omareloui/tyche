package middleware

import (
	"context"
	"net/http"

	"github.com/omareloui/tyche/internal/idgen"
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

// CorrelationIDFromCtx is the request's correlation ID, "" without one.
func CorrelationIDFromCtx(ctx context.Context) string {
	id, _ := ctx.Value(correlationIDCtxKey{}).(string)
	return id
}
