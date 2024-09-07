package handler

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-plans/internal/resperr"
)

func NotFoundHandler(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return nil, resperr.NewRespError(http.StatusNotFound)
}

func InternalServerErrorHandler(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return nil, resperr.NewRespError(http.StatusInternalServerError)
}
