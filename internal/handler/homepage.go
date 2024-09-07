package handler

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-plans/web/views"
)

func HomePageHandler(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return views.HomePage(), nil
}
