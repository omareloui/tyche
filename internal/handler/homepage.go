package handler

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-plans/web/views"
)

type HandlerMethod func(w http.ResponseWriter, r *http.Request) (templ.Component, error)

func (h Handler) HomePageHandler(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	currencies, err := h.db.GetCurrencies(r.Context())
	if err != nil {
		return nil, err
	}
	kinds, err := h.db.GetTransactionKinds(r.Context())
	if err != nil {
		return nil, err
	}
	return views.HomePage(currencies, kinds), nil
}
