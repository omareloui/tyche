package handler

import (
	"github.com/omareloui/financial-plans/internal/repository"
)

type Handler struct {
	db *repository.Repository
}

func New(repo *repository.Repository) *Handler {
	return &Handler{db: repo}
}
