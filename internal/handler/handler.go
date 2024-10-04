package handler

import (
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

type Handler struct {
	repo  *repository.Repository
	valid *valid.Validator
	sani  *sanitizer.Sanitizer
}

func New(repo *repository.Repository, valid *valid.Validator, sani *sanitizer.Sanitizer) *Handler {
	return &Handler{repo: repo, valid: valid, sani: sani}
}
