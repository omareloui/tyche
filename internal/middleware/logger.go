package middleware

import (
	"net/http"
	"time"

	"github.com/omareloui/financial-plans/internal/logger"
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

			l.Info(
				"incoming request",
				zap.String("method", r.Method),
				zap.String("url", r.URL.RequestURI()),
				zap.String("user_agent", r.UserAgent()),
				zap.Int("status_code", lrw.statusCode),
				zap.Duration("elapsed_ms", time.Since(start)),
			)
		}()

		next.ServeHTTP(lrw, r)
	})
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
