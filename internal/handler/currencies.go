package handler

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
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

	info, err := h.fxJob.Info(r.Context())
	if err != nil {
		l.Error("fetching the fx rates info", zap.Error(err))
		return nil, err
	}

	rates := views.FxRatesFreshness{Info: info, Interval: h.fxJob.Interval()}
	return views.Currencies(getClaims(r.Context()), b, currs, rates), nil
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

	claims := getClaims(r.Context())
	return views.Currency(cur, &claims.DefaultCurrency), nil
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

		claims := getClaims(r.Context())
		return views.CurrencyMovedOOB(cur, &claims.DefaultCurrency), nil
	}
}

// SetCurrencyUnit switches the unit a precious metal's amounts are shown in
// (?unit=g|oz).
func (h Handler) SetCurrencyUnit(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
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

	unit := models.MetalUnit(r.FormValue("unit"))
	if !cur.IsMetal() || (unit != models.MetalUnitGram && unit != models.MetalUnitOunce) {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Only precious metals can be shown in grams or ounces")
	}

	if err := h.repo.SetCurrencyInGrams(r.Context(), id, unit == models.MetalUnitGram); err != nil {
		l.Error("set InGrams for currency", zap.String("unit", string(unit)), zap.Error(err))
		return nil, err
	}
	l.Info("set currency unit", zap.String("currency_id", id), zap.String("unit", string(unit)))

	claims := getClaims(r.Context())
	if cur.ID == claims.DefaultCurrency.ID {
		// The claims carry the default currency, its unit with it.
		hxRespondWithRefresh(w)
		unsetCookie(w, accessTokenCookieName)
		return nil, nil
	}

	cur.InGrams = unit == models.MetalUnitGram
	return views.Currency(cur, &claims.DefaultCurrency), nil
}

func (h Handler) SetDefaultCurrency(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	if err := h.repo.SetCurrencyIsShown(r.Context(), id, true); err != nil {
		l.Error("ensure currency shown to set as default", zap.Error(err))
		return nil, err
	}
	l.Info("ensured currency is shown to set as default", zap.String("currency_id", id))

	if err := h.repo.SetDefaultCurrency(r.Context(), id); err != nil {
		l.Error("error setting default currency", zap.String("currency_id", id), zap.Error(err))
		return nil, err
	}
	l.Info("set default currency", zap.String("currency_id", id))

	hxRespondWithRefresh(w)
	unsetCookie(w, accessTokenCookieName)

	return nil, nil
}
