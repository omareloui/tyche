package handler

import (
	"github.com/omareloui/tyche/internal/badge"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/smstemplates"
	"github.com/omareloui/tyche/internal/valid"
	"github.com/omareloui/tyche/internal/vault"
)

type Handler struct {
	repo  *repository.Repository
	valid *valid.Validator
	sani  *sanitizer.Sanitizer
	fxJob *fxrate.Job
	// smsTemplates are the shared sms templates.
	smsTemplates *smstemplates.Store
	// vaults are the keys of the encrypted tenant databases.
	vaults *vault.Store
	// badges push the installed apps' badge counts.
	badges *badge.Notifier
}

func New(repo *repository.Repository, valid *valid.Validator, sani *sanitizer.Sanitizer, fxJob *fxrate.Job, smsTemplates *smstemplates.Store, badges *badge.Notifier) *Handler {
	return &Handler{repo: repo, valid: valid, sani: sani, fxJob: fxJob, smsTemplates: smsTemplates, vaults: repo.Vaults(), badges: badges}
}
