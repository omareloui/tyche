package handler

import "github.com/omareloui/financial-plans/internal/db"

type Handler struct {
	db *db.Queries
}

func NewHandler(queries *db.Queries) *Handler {
	return &Handler{db: queries}
}
