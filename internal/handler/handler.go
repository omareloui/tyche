package handler

import "github.com/omareloui/financial-plans/internal/db"

type Handler struct {
	queries *db.Queries
}

func NewHandler(queries *db.Queries) *Handler {
	return &Handler{queries: queries}
}
