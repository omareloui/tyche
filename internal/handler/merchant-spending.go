package handler

import (
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views/components"
)

func (h *Handler) MerchantSpending(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	month := r.URL.Query().Get("month")
	if _, err := time.Parse("2006-01", month); err != nil {
		return nil, resperr.New(http.StatusUnprocessableEntity, "invalid month")
	}

	ms, err := h.repo.GetMerchantSpending(r.Context(), month)
	if err != nil {
		return nil, err
	}
	return components.MerchantSpendingChart(ms), nil
}
