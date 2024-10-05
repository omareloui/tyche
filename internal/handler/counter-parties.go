package handler

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

func (h Handler) CounterParties(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}

	cp, err := h.repo.GetCounterParties(r.Context())
	if err != nil {
		l.Error("fetching counter parties", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched counter parties")

	return views.CounterParties(b, h.mapCounterPartyToFD(dfltCounterParty(), &errs.ValidationError{}), cp), nil
}

func (h Handler) CounterParty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	cp, err := h.repo.GetCounterParty(r.Context(), id)
	if err != nil {
		l.Error("fetching counterparty", zap.Error(err))
		return nil, err
	}
	l.Info("fetched counterparty", zap.Any("counterparty", cp))

	return views.CounterParty(cp), nil
}

func (h Handler) CreateCounterParty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	cp, err := h.mapFDToCounterParty(r)
	if err != nil {
		l.Error("parsing the form", zap.Error(err))
		return nil, err
	}
	cp.ID = idgen.New()

	if err := cp.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating counterparty", zap.Error(err), zap.Any("errors", valerr.Errors))
			return views.CreateCounterPartyForm(h.mapCounterPartyToFD(cp, &valerr)), nil
		}
		l.Error("sanitizing counterparty", zap.Error(err))
		return nil, err
	}

	if err := h.repo.CreateCounterParty(r.Context(), cp); err != nil {
		l.Error("creating counterparty", zap.Error(err))
		return nil, err
	}
	l.Info("created counterparty")

	if err = renderToBody(w, r, views.CounterPartyOOB(cp)); err != nil {
		l.Error("rendering counterparty oob", zap.Error(err))
		return nil, err
	}
	l.Debug("rendered counterparty oob")

	return views.CreateCounterPartyForm(h.mapCounterPartyToFD(dfltCounterParty(), &errs.ValidationError{})), nil
}

func (h Handler) GetEditCounterParty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	cp, err := h.repo.GetCounterParty(r.Context(), id)
	if err != nil {
		l.Error("fetching counterparty", zap.Error(err))
		return nil, err
	}
	l.Info("fetched counterparty", zap.Any("counterparty", cp))

	return views.EditCounterParty(id, h.mapCounterPartyToFD(cp, &errs.ValidationError{})), nil
}

func (h Handler) EditCounterParty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	cp, err := h.mapFDToCounterParty(r)
	if err != nil {
		l.Error("parsing the form", zap.Error(err))
		return nil, err
	}
	cp.ID = id

	if err := cp.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating counterparty", zap.Error(err), zap.Any("errors", valerr.Errors))
			return views.CreateCounterPartyForm(h.mapCounterPartyToFD(cp, &valerr)), nil
		}
		l.Error("sanitizing counterparty", zap.Error(err))
		return nil, err
	}

	l.Debug("update counterparty", zap.Any("counterparty", cp))

	if err := h.repo.UpdateCounterParty(r.Context(), cp); err != nil {
		l.Error("update counterparty", zap.Error(err))
		return nil, err
	}
	l.Info("updated counterparty")

	return views.CounterParty(cp), nil
}

func (h Handler) DeleteCounterParty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())
	if err := h.repo.DeleteCounterParty(r.Context(), id); err != nil {
		l.Error("deleting counterparty", zap.Error(err))
		return nil, err
	}
	l.Info("deleted counterparty")

	return nil, nil
}

func (h Handler) mapCounterPartyToFD(cp *models.CounterParty, err *errs.ValidationError) *views.CounterPartyFD {
	return &views.CounterPartyFD{
		Name: views.FDInput{Value: cp.Name, Error: err.Errors.MsgFor("Name")},
	}
}

func (h Handler) mapFDToCounterParty(r *http.Request) (*models.CounterParty, error) {
	return &models.CounterParty{
		Name: r.FormValue("name"),
	}, nil
}

func dfltCounterParty() *models.CounterParty {
	return &models.CounterParty{}
}
