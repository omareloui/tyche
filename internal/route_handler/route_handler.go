package route_handler

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/resperr"
	"go.uber.org/zap"
)

type RouteHandler func(w http.ResponseWriter, r *http.Request) (templ.Component, error)

func (fn RouteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	l := logger.FromCtx(r.Context())
	defer func() {
		if p := recover(); p != nil {
			l.Fatal("panicked", zap.Any("message", p))
			panic(p)
		}
	}()

	comp, err := fn(w, r)
	if err != nil {
		errorResponse(l, w, err)
		return
	}

	if comp != nil {
		if err := comp.Render(r.Context(), w); err != nil {
			l.Error("rendering the component")
			errorResponse(l, w, err)
			return
		}
	}

	prepareResponse(w, 0)
}

func errorResponse(l *zap.Logger, w http.ResponseWriter, err error) {
	var httperr *resperr.RespError
	if !errors.As(err, &httperr) {
		httperr = resperr.New(http.StatusInternalServerError, err.Error())
	}

	l.Error("server error",
		zap.Error(httperr),
		zap.String("message", httperr.Message),
		zap.Int("status_code", httperr.Code),
	)

	http.Error(w, httperr.Error(), httperr.Code)
}

func prepareResponse(w http.ResponseWriter, code int) {
	if code != 0 {
		w.WriteHeader(code)
	}
	w.Header().Add("Content-Type", "text/html")
}
