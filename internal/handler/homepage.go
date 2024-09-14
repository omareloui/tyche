package handler

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-planner/web/views"
)

type HandlerMethod func(w http.ResponseWriter, r *http.Request) (templ.Component, error)

func (h Handler) HomePage(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	balances, err := h.repo.GetBalances(r.Context())
	if err != nil {
		return nil, err
	}
	return views.HomePage(balances), nil
}
