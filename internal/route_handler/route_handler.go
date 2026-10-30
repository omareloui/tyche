package route_handler

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/reqpath"
	"github.com/omareloui/tyche/internal/resperr"
	"go.uber.org/zap"
)

type RouteHandler func(w http.ResponseWriter, r *http.Request) (templ.Component, error)

type JSONRouteHandler func(w http.ResponseWriter, r *http.Request) ([]byte, error)

func (fn RouteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	l := logger.FromCtx(r.Context())
	defer recoverPanic(l, w)

	comp, err := fn(w, r)
	if err != nil {
		errorResponse(l, w, err)
		return
	}

	prepareResponse(w, 0)

	if comp != nil {
		if err := comp.Render(reqpath.With(r.Context(), r.URL.Path), w); err != nil {
			l.Error("rendering the component")
			errorResponse(l, w, err)
			return
		}
	}
}

func (fn JSONRouteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	l := logger.FromCtx(r.Context())
	defer recoverPanic(l, w)

	res, err := fn(w, r)
	if err != nil {
		errorResponse(l, w, err)
		return
	}

	prepareJSONResponse(w, 0)

	_, err = w.Write(res)
	if err != nil {
		l.Error("writing the json response", zap.Error(err))
		errorResponse(l, w, err)
		return
	}
}

// recoverPanic logs a handler panic and responds with a 500 instead of
// crashing the server.
func recoverPanic(l *zap.Logger, w http.ResponseWriter) {
	if p := recover(); p != nil {
		l.Error("panicked", zap.Any("message", p), zap.Stack("stack"))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func errorResponse(l *zap.Logger, w http.ResponseWriter, err error) {
	var httperr *resperr.RespError
	if !errors.As(err, &httperr) {
		httperr = resperr.New(http.StatusInternalServerError, err.Error())
	}

	l.Warn("server error",
		zap.Error(httperr),
		zap.String("message", httperr.Message),
		zap.Int("status_code", httperr.Code),
	)

	http.Error(w, httperr.Message, httperr.Code)
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
