package handler

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
)

// MonthRecap sums up one of the user's months (?month=YYYY-MM, the last
// closed one by default) against the one before and the last months'
// average.
func (h *Handler) MonthRecap(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	claims := getClaims(ctx)
	today := h.userToday(ctx)
	p, ok := models.RecapPeriod(claims.MonthStart, r.URL.Query().Get("month"), today)
	if !ok {
		return nil, resperr.New(http.StatusUnprocessableEntity, "invalid month")
	}
	balances, err := h.repo.GetBalances(ctx)
	if err != nil {
		return nil, err
	}
	rc, err := h.repo.GetMonthRecap(ctx, p, today, &claims.DefaultCurrency)
	if err != nil {
		return nil, err
	}
	return views.MonthRecap(claims, balances, rc), nil
}
