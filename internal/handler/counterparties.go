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

func (h Handler) Counterparties(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}

	cp, err := h.repo.GetCounterparties(r.Context())
	if err != nil {
		l.Error("fetching counter parties", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched counter parties")

	return views.Counterparties(b, h.mapCounterpartyToFD(dfltCounterparty(), &errs.ValidationError{}), cp), nil
}

func (h Handler) Counterparty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	cp, err := h.repo.GetCounterparty(r.Context(), id)
	if err != nil {
		l.Error("fetching counterparty", zap.Error(err))
		return nil, err
	}
	l.Info("fetched counterparty", zap.Any("counterparty", cp))

	return views.Counterparty(cp), nil
}

func (h Handler) CreateCounterparty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	cp, err := h.mapFDToCounterparty(r)
	if err != nil {
		l.Error("parsing the form", zap.Error(err))
		return nil, err
	}
	cp.ID = idgen.New()

	if err := cp.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating counterparty", zap.Error(err), zap.Any("errors", valerr.Errors))
			return views.CreateCounterpartyForm(h.mapCounterpartyToFD(cp, &valerr)), nil
		}
		l.Error("sanitizing counterparty", zap.Error(err))
		return nil, err
	}

	if err := h.repo.CreateCounterparty(r.Context(), cp); err != nil {
		l.Error("creating counterparty", zap.Error(err))
		return nil, err
	}
	l.Info("created counterparty")

	if err = renderToBody(w, r, views.CounterpartyOOB(cp)); err != nil {
		l.Error("rendering counterparty oob", zap.Error(err))
		return nil, err
	}
	l.Debug("rendered counterparty oob")

	return views.CreateCounterpartyForm(h.mapCounterpartyToFD(dfltCounterparty(), &errs.ValidationError{})), nil
}

func (h Handler) GetEditCounterparty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	cp, err := h.repo.GetCounterparty(r.Context(), id)
	if err != nil {
		l.Error("fetching counterparty", zap.Error(err))
		return nil, err
	}
	l.Info("fetched counterparty", zap.Any("counterparty", cp))

	return views.EditCounterparty(id, h.mapCounterpartyToFD(cp, &errs.ValidationError{})), nil
}

func (h Handler) EditCounterparty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	cp, err := h.mapFDToCounterparty(r)
	if err != nil {
		l.Error("parsing the form", zap.Error(err))
		return nil, err
	}
	cp.ID = id

	if err := cp.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating counterparty", zap.Error(err), zap.Any("errors", valerr.Errors))
			return views.CreateCounterpartyForm(h.mapCounterpartyToFD(cp, &valerr)), nil
		}
		l.Error("sanitizing counterparty", zap.Error(err))
		return nil, err
	}

	l.Debug("update counterparty", zap.Any("counterparty", cp))

	if err := h.repo.UpdateCounterparty(r.Context(), cp); err != nil {
		l.Error("update counterparty", zap.Error(err))
		return nil, err
	}
	l.Info("updated counterparty")

	return views.Counterparty(cp), nil
}

func (h Handler) DeleteCounterparty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())
	if err := h.repo.DeleteCounterparty(r.Context(), id); err != nil {
		l.Error("deleting counterparty", zap.Error(err))
		return nil, err
	}
	l.Info("deleted counterparty")

	return nil, nil
}

func (h Handler) mapCounterpartyToFD(cp *models.Counterparty, err *errs.ValidationError) *views.CounterpartyFD {
	return &views.CounterpartyFD{
		Name: views.FDInput{Value: cp.Name, Error: err.Errors.MsgFor("Name")},
	}
}

func (h Handler) mapFDToCounterparty(r *http.Request) (*models.Counterparty, error) {
	return &models.Counterparty{
		Name: r.FormValue("name"),
	}, nil
}

func dfltCounterparty() *models.Counterparty {
	return &models.Counterparty{}
}
