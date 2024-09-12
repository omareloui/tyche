package handler

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-plans/internal/resperr"
)

func (h Handler) NotFound(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return nil, resperr.NewRespError(http.StatusNotFound)
}

func (h Handler) InternalServerError(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return nil, resperr.NewRespError(http.StatusInternalServerError)
}
