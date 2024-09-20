package handler

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-planner/internal/resperr"
)

func (h Handler) NotFound(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return nil, resperr.New(http.StatusNotFound, "")
}

func (h Handler) InternalServerError(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return nil, resperr.New(http.StatusInternalServerError, "")
}
