package route_handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/reqpath"
	"github.com/omareloui/tyche/internal/resperr"
	"go.uber.org/zap"
)

type RouteHandler func(w http.ResponseWriter, r *http.Request) (templ.Component, error)

type JSONRouteHandler func(w http.ResponseWriter, r *http.Request) ([]byte, error)

func (fn RouteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	l := logger.FromCtx(r.Context())
	defer recoverPanic(l, w, r)

	comp, err := fn(w, r)
	if err != nil {
		errorResponse(l, w, r, err)
		return
	}

	prepareResponse(w, 0)

	if comp != nil {
		if err := comp.Render(reqpath.With(r.Context(), r.URL.Path), w); err != nil {
			l.Error("rendering the component", zap.Error(err))
			errorResponse(l, w, r, err)
			return
		}
	}
}

func (fn JSONRouteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	l := logger.FromCtx(r.Context())
	defer recoverPanicJSON(l, w)

	res, err := fn(w, r)
	if err != nil {
		jsonErrorResponse(l, w, err)
		return
	}

	prepareJSONResponse(w, 0)

	_, err = w.Write(res)
	if err != nil {
		l.Error("writing the json response", zap.Error(err))
		return
	}
}

// recoverPanic logs a handler panic and responds with a 500 instead of
// crashing the server.
func recoverPanic(l *zap.Logger, w http.ResponseWriter, r *http.Request) {
	if p := recover(); p != nil {
		l.Error("panicked", zap.Any("message", p), zap.Stack("stack"))
		middleware.WriteError(w, r, http.StatusInternalServerError, "")
	}
}

func recoverPanicJSON(l *zap.Logger, w http.ResponseWriter) {
	if p := recover(); p != nil {
		l.Error("panicked", zap.Any("message", p), zap.Stack("stack"))
		writeJSONError(w, http.StatusInternalServerError, "")
	}
}

// respErr is the response error of err, a 500 when it isn't one.
func respErr(l *zap.Logger, err error) *resperr.RespError {
	var httperr *resperr.RespError
	if !errors.As(err, &httperr) {
		httperr = resperr.New(http.StatusInternalServerError, err.Error())
	}

	l.Warn("server error",
		zap.Error(httperr),
		zap.String("message", httperr.Message),
		zap.Int("status_code", httperr.Code),
	)
	return httperr
}

func errorResponse(l *zap.Logger, w http.ResponseWriter, r *http.Request, err error) {
	httperr := respErr(l, err)
	middleware.WriteError(w, r, httperr.Code, httperr.Message)
}

func jsonErrorResponse(l *zap.Logger, w http.ResponseWriter, err error) {
	httperr := respErr(l, err)
	writeJSONError(w, httperr.Code, httperr.Message)
}

// writeJSONError writes {"error": msg}, the status text for 5xx (their
// message is for the logs only).
func writeJSONError(w http.ResponseWriter, code int, msg string) {
	if msg == "" || code >= 500 {
		msg = http.StatusText(code)
	}
	body, _ := json.Marshal(map[string]string{"error": msg})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(body)
}

func prepareResponse(w http.ResponseWriter, code int) {
	if code != 0 {
		w.WriteHeader(code)
	}
	w.Header().Add("Content-Type", "text/html")
}

func prepareJSONResponse(w http.ResponseWriter, code int) {
	if code != 0 {
		w.WriteHeader(code)
	}
	w.Header().Add("Content-Type", "application/json")
}
