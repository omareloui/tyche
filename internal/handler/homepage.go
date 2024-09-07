package handler

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-plans/web/views"
)

func HomePageHandler(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	monthlyIncome := "14000"
	monthlyExpenses := "6500"

	return views.HomePage(monthlyIncome, monthlyExpenses), nil
}
