package handler

import (
	"github.com/omareloui/financial-plans/internal/repository"
	"github.com/omareloui/financial-plans/internal/sanitizer"
	"github.com/omareloui/financial-plans/internal/valid"
)

type Handler struct {
	repo  *repository.Repository
	valid *valid.Validator
	sani  *sanitizer.Sanitizer
}

func New(repo *repository.Repository, valid *valid.Validator, sani *sanitizer.Sanitizer) *Handler {
	return &Handler{repo: repo, valid: valid, sani: sani}
}
