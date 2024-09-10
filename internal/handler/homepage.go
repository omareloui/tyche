package handler

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-plans/internal/db"
	"github.com/omareloui/financial-plans/web/views"
)

func (h Handler) HomePageHandler(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	monthlyIncome := "14000"
	monthlyExpenses := "6500"

	createdAuthor, _ := h.db.CreateAuthor(r.Context(),
		db.CreateAuthorParams{
			Name: "Omar Eloui",
			Bio:  sql.NullString{String: "This is my bio"},
		})
	fmt.Println(createdAuthor)

	author, err := h.db.GetAuthor(r.Context(), createdAuthor.ID)

	fmt.Println(author, err)

	return views.HomePage(monthlyIncome, monthlyExpenses), nil
}
