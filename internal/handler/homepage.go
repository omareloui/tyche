package handler

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-plans/web/views"
)

type HandlerMethod func(w http.ResponseWriter, r *http.Request) (templ.Component, error)

func (h Handler) HomePage(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return views.HomePage(), nil
}
