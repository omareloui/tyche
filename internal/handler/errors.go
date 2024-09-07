package handler

import (
	"io"
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-plans/internal/resperr"
)

func NotFoundHandler(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return nil, resperr.NewRespError(http.StatusNotFound)
}

func InternalServerErrorHandler(w http.ResponseWriter, r *http.Request) {
	errorHandler(w, r, http.StatusInternalServerError)
}

func errorHandler(w http.ResponseWriter, _ *http.Request, code int) {
	w.Header().Add("Content-Type", "text/html")
	w.WriteHeader(code)
	_, err := io.WriteString(w, http.StatusText(code))
	if err != nil {
		return
	}
}
