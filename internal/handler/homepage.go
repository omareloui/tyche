package handler

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/web/views"
)

func (h Handler) HomePage(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	claims := getClaims(r.Context())

	if claims == nil {
		return views.HomePage(claims, []models.Balance{}), nil
	}

	balances, err := h.repo.GetBalances(r.Context())
	if err != nil {
		return nil, err
	}
	return views.HomePage(claims, balances), nil
}
