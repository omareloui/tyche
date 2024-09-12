package handler

import (
	"github.com/omareloui/financial-plans/internal/repository"
	"github.com/omareloui/financial-plans/internal/valid"
)

type Handler struct {
	repo  *repository.Repository
	valid *valid.Validator
}

func New(repo *repository.Repository, valid *valid.Validator) *Handler {
	return &Handler{repo: repo, valid: valid}
}
