package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

const generateNextScheduledTransactionsCount = 5

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

	accs, knds, cats, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	ftx, err := h.repo.GetFrequentTx(r.Context(), id)
	if err != nil {
		l.Error("fetching frequent tx", zap.Error(err))
		return nil, err
	}
	l.Info("fetched frequent tx")

	return views.EditFrequentTx(ftx,
		h.mapFrequentTxToFD(ftx, &errs.ValidationError{}),
		accs, knds, getCategoriesOfKinds(cats, ftx.Kind.ID)), nil
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

	ftx.LastTransactionCreatedAt = time.Now()
	scheduled := generateNextScheduledTransactions(ftx, generateNextScheduledTransactionsCount)

	if err := h.repo.UpdateFrequentTx(r.Context(), ftx, scheduled); err != nil {
		l.Error("updating frequent tx", zap.Error(err))
		return nil, err
	}
	l.Info("updated frequent tx", zap.Any("frequent_tx", ftx))

	ftx, err = h.repo.GetFrequentTx(r.Context(), id)
	if err != nil {
		l.Error("fetching frequent tx", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched frequent tx", zap.Any("frequent_tx", ftx))

	return views.FrequentTx(ftx), nil
}

func (h Handler) GenerateTransactions(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
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
	l.Debug("fetched frequent tx", zap.Any("frequent tx", ftx))

	scheduled := generateNextScheduledTransactions(ftx, generateNextScheduledTransactionsCount)

	if err := h.repo.GenerateScheduledTransactions(r.Context(), ftx, scheduled); err != nil {
		return nil, err
	}

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

func generateNextScheduledTransactions(ftx *models.FrequentTx, count int) []models.Transaction {
	txs := []models.Transaction{}
	now := time.Now()

	for range count {
		var nextOn time.Time

		switch ftx.FrequencyBy {
		case models.Day:
			nextOn = ftx.LastTransactionCreatedAt.AddDate(0, 0, int(ftx.Frequency))
		case models.Month:
			nextOn = ftx.LastTransactionCreatedAt.AddDate(0, int(ftx.Frequency), 0)
			if d := nextOn.Day(); d != ftx.LastTransactionCreatedAt.Day() {
				nextOn = nextOn.AddDate(0, 0, -d)
			}
		case models.Year:
			nextOn = ftx.LastTransactionCreatedAt.AddDate(int(ftx.Frequency), 0, 0)
			if d := nextOn.Day(); d != ftx.LastTransactionCreatedAt.Day() {
				nextOn = nextOn.AddDate(0, 0, -d)
			}
		}

		nextOn = nextOn.Truncate(24 * time.Hour)

		if !ftx.StopsOn.IsZero() && nextOn.After(ftx.StopsOn) {
			break
		}

		txs = append(txs, models.Transaction{
			ID:          idgen.New(),
			Account:     ftx.Account,
			Amount:      ftx.Amount,
			Kind:        ftx.Kind,
			Category:    ftx.Category,
			Description: ftx.Description,
			ScheduledTo: nextOn,
			CreatedAt:   now,
		})
		ftx.LastTransactionCreatedAt = nextOn
	}
	return txs
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
		Account: models.Account{
			ID: r.FormValue("account_id"),
		},
		Amount:       amount,
		Frequency:    uint(freq),
		FrequencyBy:  models.FrequencyBy(freqBy),
		StopsOn:      stopsOn,
		Transactions: []models.Transaction{},
		Description:  r.FormValue("description"),
		Kind:         models.TransactionKind{ID: r.FormValue("kind_id")},
		Category:     &models.Category{ID: r.FormValue("category_id")},
	}, nil
}

func (h Handler) mapFrequentTxToFD(ftx *models.FrequentTx, valerr *errs.ValidationError) *views.FrequentTxFD {
	var catId string
	var toAccId string

	if ftx.Category != nil {
		catId = ftx.Category.ID
	}

	if ftx.ToAccount != nil {
		toAccId = ftx.ToAccount.ID
	}

	return &views.FrequentTxFD{
		FrequencyFD: views.FrequencyFD{
			IsFrequent: views.FDInput{Value: "on"},
			FrequentBy: views.FDInput{Value: formatIntIfNonZero(int(ftx.FrequencyBy)), Error: valerr.Errors.MsgFor("FrequentBy")},
			Frequency:  views.FDInput{Value: formatIntIfNonZero(int(ftx.Frequency)), Error: valerr.Errors.MsgFor("Frequency")},
			StopsOn:    views.FDInput{Value: formatDateOnlyIfNonZero(ftx.StopsOn), Error: valerr.Errors.MsgFor("StopsOn")},
		},
		TransactionAmountsAndKindFD: views.TransactionAmountsAndKindFD{
			AccountIFD: views.AccountIFD{
				AccountID: views.FDInput{Value: ftx.Account.ID, Error: valerr.Errors.MsgFor("Account.ID")},
				Amount:    views.FDInput{Value: formatFloatIfNonZero(ftx.Amount), Error: valerr.Errors.MsgFor("Amount")},
			},
			KindID: views.FDInput{Value: ftx.Kind.ID, Error: valerr.Errors.MsgFor("Kind.ID")},
			TransferAccountIFD: &views.AccountIFD{
				AccountID: views.FDInput{Value: toAccId, Error: valerr.Errors.MsgFor("ToAccount.ID")},
				Amount:    views.FDInput{Value: formatFloatIfNonZero(ftx.ToAmount), Error: valerr.Errors.MsgFor("ToAmount")},
			},
			CategoryID: &views.FDInput{Value: catId, Error: valerr.Errors.MsgFor("Category.ID")},
		},
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
