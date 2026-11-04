package handler

import (
	"net/http"
	"slices"
	"strings"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

func (h Handler) AdminFxRates(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}

	currs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}

	status, err := h.fxRatesStatus(r)
	if err != nil {
		return nil, err
	}

	return views.AdminFxRates(getClaims(r.Context()), b, status, currs), nil
}

func (h Handler) AdminFxRatesStatus(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	status, err := h.fxRatesStatus(r)
	if err != nil {
		return nil, err
	}
	return views.FxRatesStatus(status), nil
}

func (h Handler) RefreshFxRates(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	queued := h.fxJob.Trigger()
	l.Info("triggered the fx rates job", zap.Bool("queued", queued))

	status, err := h.fxRatesStatus(r)
	if err != nil {
		return nil, err
	}
	// The run starts in the background; show it as pending right away so the
	// status keeps polling until it's done.
	status.Pending = queued
	return views.FxRatesStatus(status), nil
}

func (h Handler) fxRatesStatus(r *http.Request) (views.FxRatesStatusData, error) {
	info, err := h.fxJob.Info(r.Context())
	if err != nil {
		logger.FromCtx(r.Context()).Error("fetching the fx rates info", zap.Error(err))
		return views.FxRatesStatusData{}, err
	}
	return views.FxRatesStatusData{
		Info:     info,
		Status:   h.fxJob.Status(),
		Interval: h.fxJob.Interval(),
	}, nil
}

func (h Handler) AdminTenants(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		return nil, err
	}
	data, err := h.adminTenantsData(r)
	if err != nil {
		return nil, err
	}
	return views.AdminTenants(getClaims(r.Context()), b, data), nil
}

// DeleteTenant deletes a user's account, e.g. when they forgot the password
// of their encrypted database (and its recovery code) or it's broken: they
// start over by signing in again.
func (h Handler) DeleteTenant(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	id := r.PathValue("id")
	if !repository.ValidTenantID(id) {
		return nil, resperr.New(http.StatusNotFound, "No such account.")
	}

	data, err := h.adminTenantsData(r)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(data.Tenants, func(t models.TenantInfo) bool { return t.ID == id }) {
		return nil, resperr.New(http.StatusNotFound, "No such account.")
	}
	if id == data.Self {
		return nil, resperr.New(http.StatusUnprocessableEntity, "That's your own account, use Reset account instead.")
	}
	if strings.TrimSpace(r.FormValue("confirm")) != id {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Type the account ID to confirm.")
	}

	if err := h.repo.DeleteTenant(ctx, id); err != nil {
		logger.FromCtx(ctx).Error("deleting a tenant", zap.String("tenantID", id), zap.Error(err))
		return nil, err
	}
	forgetUnlocks(id)
	if err := h.badges.Forget(ctx, id); err != nil {
		logger.FromCtx(ctx).Error("forgetting a tenant's push subscriptions", zap.String("tenantID", id), zap.Error(err))
	}

	data, err = h.adminTenantsData(r)
	if err != nil {
		return nil, err
	}
	data.Deleted = id
	// The form targets its error message, the list replaces itself.
	w.Header().Set("HX-Retarget", "#admin-tenants")
	w.Header().Set("HX-Reswap", "outerHTML")
	return views.AdminTenantsList(data), nil
}

func (h Handler) adminTenantsData(r *http.Request) (views.AdminTenantsData, error) {
	tenants, err := h.repo.ListTenants(r.Context())
	if err != nil {
		logger.FromCtx(r.Context()).Error("listing the tenants", zap.Error(err))
		return views.AdminTenantsData{}, err
	}
	claims := getClaims(r.Context())
	return views.AdminTenantsData{
		Tenants: tenants,
		Self:    models.TenantID(claims.OAuthProvider, claims.OAuthID),
	}, nil
}
