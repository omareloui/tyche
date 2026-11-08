package middleware

import (
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"go.uber.org/zap"
)

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		l := logger.Get()

		if correlationID, ok := r.Context().Value(correlationIDCtxKey{}).(string); ok {
			l = l.With(zap.String("correlation_id", correlationID))
		}

		lrw := newLoggingResponseWriter(w)

		r = r.WithContext(logger.WithContext(r.Context(), l))

		defer func() {
			panicVal := recover()
			if panicVal != nil {
				lrw.statusCode = http.StatusInternalServerError
				panic(panicVal)
			}

			fields := []zap.Field{
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status_code", lrw.statusCode),
				// zap.Duration would write seconds in the JSON file.
				zap.Float64("elapsed_ms", float64(time.Since(start).Microseconds())/1000),
			}
			if keys := queryKeys(r.URL.Query()); keys != "" {
				fields = append(fields, zap.String("query", keys))
			}
			l.Info("incoming request", fields...)
		}()

		next.ServeHTTP(lrw, r)
	})
}

// queryKeys are the query's keys, sorted: never its values, which hold
// searches, amounts and the OAuth code.
func queryKeys(q url.Values) string {
	return strings.Join(slices.Sorted(maps.Keys(q)), ",")
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newLoggingResponseWriter(w http.ResponseWriter) *loggingResponseWriter {
	return &loggingResponseWriter{w, http.StatusOK}
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}
