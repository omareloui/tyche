package handler

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-planner/internal/logger"
	"github.com/omareloui/financial-planner/web/views"
	"go.uber.org/zap"
)

func (h Handler) Goals(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}

	return views.Goals(b), nil
}

func (h Handler) CreateGoals(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return nil, nil
}
