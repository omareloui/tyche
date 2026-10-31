package handler

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/logger"
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
