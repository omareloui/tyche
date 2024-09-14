package handler

import (
	"github.com/omareloui/financial-planner/internal/repository"
	"github.com/omareloui/financial-planner/internal/sanitizer"
	"github.com/omareloui/financial-planner/internal/valid"
)

type Handler struct {
	repo  *repository.Repository
	valid *valid.Validator
	sani  *sanitizer.Sanitizer
}

func New(repo *repository.Repository, valid *valid.Validator, sani *sanitizer.Sanitizer) *Handler {
	return &Handler{repo: repo, valid: valid, sani: sani}
}
