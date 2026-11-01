package handler

import (
	"net/http"
	"strings"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
)

func (h *Handler) ResetPage(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		return nil, err
	}
	return views.Reset(getClaims(r.Context()), b), nil
}

func (h *Handler) Reset(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	if err := r.ParseForm(); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "invalid form")
	}

	opts := models.ParseResetOptions(r.Form)
	if opts.IsEmpty() {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Select what to delete.")
	}
	if strings.TrimSpace(r.FormValue("confirm")) != models.ResetConfirmation {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Type "+models.ResetConfirmation+" to confirm.")
	}

	if err := h.repo.Reset(r.Context(), opts); err != nil {
		return nil, err
	}
	if opts.Has(models.ResetDevices) {
		tenantID, err := repository.TenantIDFromCtx(r.Context())
		if err != nil {
			return nil, err
		}
		if err := h.vaults.DeleteDevices(r.Context(), tenantID); err != nil {
			return nil, err
		}
	}

	// The access token carries the default currency and account, get a new
	// one with the reset ones.
	unsetCookie(w, accessTokenCookieName)
	w.Header().Set("HX-Redirect", "/")
	return nil, nil
}
