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

	ftxs, err := h.repo.GetFrequentTxs(r.Context())
	if err != nil {
		return nil, err
	}
	l.Info("fetched frequent transactions")

	return views.FrequentTxs(balances, ftxs), nil
}

func (h Handler) FrequentTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	ftx, err := h.repo.GetFrequentTx(r.Context(), id)
	if err != nil {
		l.Error("fetching frequent tx", zap.Error(err))
		return nil, err
	}
	l.Info("fetched frequent tx", zap.Any("frequent tx", ftx))

	return views.FrequentTx(ftx), nil
}

func (h Handler) GetEditFrequentTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	currencies, kinds, err := h.getCurrenciesAndKinds(r.Context())
	if err != nil {
		return nil, err
	}

	ftx, err := h.repo.GetFrequentTx(r.Context(), id)
	if err != nil {
		l.Error("fetching frequent tx", zap.Error(err))
		return nil, err
	}
	l.Info("fetched frequent tx", zap.Any("frequent tx", ftx))

	return views.EditFrequentTx(ftx, h.mapFrequentTxToFD(ftx, &errs.ValidationError{}), h.repo.CurrenciesToMap(currencies), h.repo.TransactionKindsToMap(kinds)), nil
}

func (h Handler) EditFrequentTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())
	ftx, err := h.mapFDToFrequentTx(r)
	if err != nil {
		l.Error("parsing formdata", zap.Error(err))
		return nil, err
	}
	ftx.ID = id

	if err := h.repo.UpdateFrequentTx(r.Context(), ftx); err != nil {
		l.Error("updating frequent tx", zap.Error(err))
		return nil, err
	}
	l.Info("updated frequent tx", zap.Any("frequent tx", ftx))

	ftx, err = h.repo.GetFrequentTx(r.Context(), id)
	if err != nil {
		l.Error("fetching frequent tx", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched frequent tx", zap.Any("frequent tx", ftx))

	return views.FrequentTx(ftx), nil
}

func (h Handler) DeleteFrequentTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	if err := h.repo.DeleteFrequentTx(r.Context(), id); err != nil {
		l.Error("deleting frequent transaction", zap.Error(err))
		return nil, err
	}
	l.Info("deleted frequent transaction")

	return nil, nil
}

func generateScheduledTransactionsUntilNextYear(*models.FrequentTx) []models.Transaction {
	// TODO:
	return nil
}

func (h Handler) mapFDToFrequentTx(r *http.Request) (*models.FrequentTx, error) {
	amount, err := parseFloatIfExists(r.FormValue("amount"))
	if err != nil {
		return nil, err
	}

	stopsOn, err := parseDateOnlyIfExists(r.FormValue("stops_on"))
	if err != nil {
		return nil, err
	}

	freqBy, err := parseIntIfExists(r.FormValue("frequent_by"))
	if err != nil {
		return nil, err
	}

	freq, err := parseUintIfExists(r.FormValue("frequency"))
	if err != nil {
		return nil, err
	}

	return &models.FrequentTx{
		Frequency:    uint(freq),
		FrequencyBy:  models.FrequencyBy(freqBy),
		StopsOn:      stopsOn,
		Transactions: []models.Transaction{},
		Description:  r.FormValue("description"),
		Amount:       amount,
		Currency:     models.Currency{ID: r.FormValue("currency_id")},
		Kind:         models.TransactionKind{ID: r.FormValue("kind_id")},
	}, nil
}

func (h Handler) mapFrequentTxToFD(ftx *models.FrequentTx, valerr *errs.ValidationError) *views.FrequentTxFD {
	return &views.FrequentTxFD{
		AmountFD: views.AmountFD{
			Amount:     views.FDInput{Value: formatIntIfNonZero(int(ftx.Amount)), Error: valerr.Errors.MsgFor("Amount")},
			CurrencyID: views.FDInput{Value: ftx.Currency.ID, Error: valerr.Errors.MsgFor("Currency.ID")},
		},
		FrequencyFD: views.FrequencyFD{
			IsFrequent: views.FDInput{Value: "on"},
			FrequentBy: views.FDInput{Value: formatIntIfNonZero(int(ftx.FrequencyBy)), Error: valerr.Errors.MsgFor("FrequentBy")},
			Frequency:  views.FDInput{Value: formatIntIfNonZero(int(ftx.Frequency)), Error: valerr.Errors.MsgFor("Frequency")},
			StopsOn:    views.FDInput{Value: formatDateOnlyIfNonZero(ftx.StopsOn), Error: valerr.Errors.MsgFor("StopsOn")},
		},
		KindID:      views.FDInput{Value: ftx.Kind.ID, Error: valerr.Errors.MsgFor("KindID")},
		Description: views.FDInput{Value: ftx.Description, Error: valerr.Errors.MsgFor("Description")},
	}
}

func (h Handler) parseFrequencyFD(r *http.Request, err *errs.ValidationError) *views.FrequencyFD {
	return &views.FrequencyFD{
		IsFrequent: views.FDInput{Value: r.FormValue("is_frequent"), Error: err.Errors.MsgFor("IsFrequent")},
		FrequentBy: views.FDInput{Value: r.FormValue("frequent_by"), Error: err.Errors.MsgFor("FrequentBy")},
		Frequency:  views.FDInput{Value: r.FormValue("frequency"), Error: err.Errors.MsgFor("Frequency")},
		StopsOn:    views.FDInput{Value: r.FormValue("stops_on"), Error: err.Errors.MsgFor("StopsOn")},
	}
}
