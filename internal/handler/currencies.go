package handler

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

func (h Handler) Currencies(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}

	currs, err := h.repo.GetAllCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched currencies")

	return views.Currencies(getClaims(r), b, currs), nil
}

func (h Handler) Currency(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	cur, err := h.repo.GetCurrency(r.Context(), id)
	if err != nil {
		l.Error("fetching currency", zap.Error(err))
		return nil, err
	}
	l.Info("fetched currency", zap.Any("currency", cur))

	return views.Currency(cur), nil
}

func (h Handler) ChangeIsShownForCurrency(isShown bool) func(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return func(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
		id := r.PathValue("id")
		if id == "" {
			return nil, errs.ErrInvalidID
		}

		l := logger.FromCtx(r.Context())

		if err := h.repo.SetCurrencyIsShown(r.Context(), id, isShown); err != nil {
			l.Error("set IsShown for currency", zap.Bool("is_shown", isShown), zap.Error(err))
			return nil, err
		}
		l.Info("set IsShown for currency", zap.Bool("is_shown", isShown))

		cur, err := h.repo.GetCurrency(r.Context(), id)
		if err != nil {
			l.Error("fetching currency", zap.Error(err))
			return nil, err
		}
		l.Debug("fetched currency", zap.Any("currency", cur))

		return views.Currency(cur), nil
	}
}

func (h Handler) SetDefaultCurrency(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	if err := h.repo.SetDefaultCurrency(r.Context(), id); err != nil {
		l.Error("error setting default currency", zap.String("currency_id", id), zap.Error(err))
		return nil, err
	}

	hxRespondWithRefresh(w)

	return nil, nil
}
