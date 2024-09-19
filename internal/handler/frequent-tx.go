package handler

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-planner/internal/errs"
	"github.com/omareloui/financial-planner/internal/logger"
	"github.com/omareloui/financial-planner/internal/models"
	"github.com/omareloui/financial-planner/web/views"
	"go.uber.org/zap"
)

func (h Handler) FrequencyForm(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	id := r.PathValue("id")
	fd := h.parseFrequencyFD(r, &errs.ValidationError{})

	if fd.FrequentBy.Value == "" {
		fd.FrequentBy.Value = fmt.Sprintf("%d", models.Month)
		fd.Frequency.Value = "1"
	}

	l.Info("responding with", zap.Any("formdata", fd))

	return views.FrequencyFormBody(id, fd), nil
}

func (h Handler) FrequentTxs(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	balances, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched balances")

	ftxs, err := h.repo.FrequentTxs(r.Context())
	if err != nil {
		return nil, err
	}
	l.Info("fetched frequent transactions")

	return views.FrequentTxs(balances, ftxs), nil
}

func (h Handler) FrequentTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return nil, nil
}

func (h Handler) GetEditFrequentTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return nil, nil
}

func (h Handler) EditFrequentTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return nil, nil
}

func (h Handler) DeleteFrequentTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return nil, nil
}

func (h Handler) parseFrequencyFD(r *http.Request, err *errs.ValidationError) *views.FrequencyFD {
	return &views.FrequencyFD{
		IsFrequent: views.FDInput{Value: r.FormValue("is_frequent"), Error: err.Errors.MsgFor("IsFrequent")},
		FrequentBy: views.FDInput{Value: r.FormValue("frequent_by"), Error: err.Errors.MsgFor("FrequentBy")},
		Frequency:  views.FDInput{Value: r.FormValue("frequency"), Error: err.Errors.MsgFor("Frequency")},
		StopsOn:    views.FDInput{Value: r.FormValue("stops_on"), Error: err.Errors.MsgFor("StopsOn")},
	}
}

func (h Handler) mapFDToTransactionFrequency(r *http.Request, parsedTx *models.Transaction) (*models.FrequentTx, error) {
	frequency, err := parseIntIfExists(r.FormValue("frequency"))
	if err != nil {
		return nil, err
	}

	frequencyBy, err := parseIntIfExists(r.FormValue("frequent_by"))
	if err != nil {
		return nil, err
	}

	stopsOn, err := parseDateOnlyIfExists(r.FormValue("stops_on"))
	if err != nil {
		return nil, err
	}

	return &models.FrequentTx{
		Amount:      parsedTx.Amount,
		Currency:    models.Currency{ID: parsedTx.Currency.ID},
		Kind:        models.TransactionKind{ID: parsedTx.Kind.ID},
		Description: parsedTx.Description,
		FrequencyBy: models.FrequencyBy(frequencyBy),
		Frequency:   uint(frequency),
		StopsOn:     stopsOn,
	}, nil
}
