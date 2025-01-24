package handler

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

const generateNextScheduledTransactionsCount = 5

func (h Handler) FrequencyForm(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	id := r.PathValue("id")
	fd := h.parseFrequencyFD(r, &errs.ValidationError{})

	if fd.FrequentBy.Value == "" {
		fd.FrequentBy.Value = fmt.Sprintf("%d", models.FreqbyMonth)
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

	return views.FrequentTxs(getClaims(r), balances, ftxs), nil
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

	accs, knds, cats, cps, gls, err := h.getTxDeps(r.Context())
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
		h.mapFrequentTxToFD(ftx, &errs.ValidationError{}, knds),
		accs, knds, cats, cps, gls), nil
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
	ftx.LastTransactionScheduledTo = time.Now()

	accs, knds, cats, cps, gls, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	if knd := slicesutils.Find(knds, func(k models.TransactionKind) bool { return k.ID == ftx.Kind.ID }); knd != nil {
		ftx.Kind = *knd
	} else {
		l.Error("mapping kind id to frequent transaction")
		return nil, errs.ErrInvalidID
	}

	oldFtx, err := h.repo.GetFrequentTx(r.Context(), id)
	if err != nil {
		l.Error("getting old frequent tx", zap.String("id", id), zap.Error(err))
		return nil, err
	}

	ftx.StartsAt = oldFtx.StartsAt

	if err := ftx.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating frequent transaction", zap.Error(valerr), zap.Any("errors", valerr.Errors))
			return views.EditFrequentTx(ftx, h.mapFrequentTxToFD(ftx, &valerr, knds),
				accs, knds, cats, cps, gls), nil
		}
		l.Error("sanitizing frequent transaction", zap.Error(err))
		return nil, err
	}

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
		case models.FreqByDay:
			nextOn = ftx.LastTransactionScheduledTo.AddDate(0, 0, int(ftx.Frequency))
		case models.FreqbyMonth:
			nextOn = ftx.LastTransactionScheduledTo.AddDate(0, int(ftx.Frequency), 0)
			if d := nextOn.Day(); d != ftx.LastTransactionScheduledTo.Day() {
				nextOn = nextOn.AddDate(0, 0, -d)
			}
		case models.FreqByYear:
			nextOn = ftx.LastTransactionScheduledTo.AddDate(int(ftx.Frequency), 0, 0)
			if d := nextOn.Day(); d != ftx.LastTransactionScheduledTo.Day() {
				nextOn = nextOn.AddDate(0, 0, -d)
			}
		}

		nextOn = nextOn.Truncate(24 * time.Hour)

		if !ftx.StopsAt.IsZero() && nextOn.After(ftx.StopsAt) {
			break
		}

		txs = append(txs, models.Transaction{
			ID:           idgen.New(),
			Account:      ftx.Account,
			Amount:       ftx.Amount,
			Kind:         ftx.Kind,
			Category:     ftx.Category,
			Counterparty: ftx.Counterparty,
			ToAccount:    ftx.ToAccount,
			ToAmount:     ftx.ToAmount,
			Description:  ftx.Description,
			ScheduledTo:  nextOn,
			CreatedAt:    now,
		})
		ftx.LastTransactionScheduledTo = nextOn
	}
	return txs
}

func (h Handler) mapFDToFrequentTx(r *http.Request) (*models.FrequentTx, error) {
	amount, err := parseFloatIfExists(r.FormValue("amount"))
	if err != nil {
		return nil, err
	}

	toAmount, err := parseFloatIfExists(r.FormValue("to_amount"))
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

	kndId := r.FormValue("kind_id")

	var cat *models.Category
	if catId := r.FormValue("category_id"); catId != "" {
		cat = &models.Category{ID: catId, Kind: models.TransactionKind{ID: kndId}}
	}

	var cp *models.Counterparty
	if cpId := r.FormValue("counterparty_id"); cpId != "" {
		cp = &models.Counterparty{ID: cpId}
	}

	toAccId := r.FormValue("to_account_id")
	var toAcc *models.Account
	if toAccId != "" {
		toAcc = &models.Account{ID: toAccId}
	}

	glId := r.FormValue("goal_id")
	var gl *models.Goal
	if glId != "" {
		gl = &models.Goal{ID: glId}
	}

	return &models.FrequentTx{
		Account:      models.Account{ID: r.FormValue("account_id")},
		Amount:       amount,
		Frequency:    uint(freq),
		FrequencyBy:  models.FrequencyBy(freqBy),
		StopsAt:      stopsOn,
		Transactions: []models.Transaction{},
		Description:  r.FormValue("description"),
		Kind:         models.TransactionKind{ID: kndId},
		Goal:         gl,
		Category:     cat,
		Counterparty: cp,
		ToAccount:    toAcc,
		ToAmount:     toAmount,
	}, nil
}

func (h Handler) mapFrequentTxToFD(ftx *models.FrequentTx, valerr *errs.ValidationError, knds []models.TransactionKind) *views.FrequentTxFD {
	fd := &views.FrequentTxFD{
		FrequencyFD: views.FrequencyFD{
			IsFrequent: views.FDInput{Value: "on"},
			FrequentBy: views.FDInput{Value: formatIntIfNonZero(int(ftx.FrequencyBy)), Error: valerr.Errors.MsgFor("FrequentBy")},
			Frequency:  views.FDInput{Value: formatIntIfNonZero(int(ftx.Frequency)), Error: valerr.Errors.MsgFor("Frequency")},
			StopsOn:    views.FDInput{Value: formatDateOnlyIfNonZero(ftx.StopsAt), Error: valerr.Errors.MsgFor("StopsOn")},
		},
		TransactionAmountsAndKindFD: views.TransactionAmountsAndKindFD{
			AccountIFD: views.AccountIFD{
				AccountID: views.FDInput{Value: ftx.Account.ID, Error: valerr.Errors.MsgFor("AccountID")},
				Amount:    views.FDInput{Value: formatFloatIfNonZero(ftx.Amount), Error: valerr.Errors.MsgFor("Amount")},
			},
			KindID: views.FDInput{Value: ftx.Kind.ID, Error: valerr.Errors.MsgFor("KindID")},
		},
		Description: views.FDInput{Value: ftx.Description, Error: valerr.Errors.MsgFor("Description")},
	}

	knd := slicesutils.Find(knds, func(k models.TransactionKind) bool { return k.ID == ftx.Kind.ID })
	hasToAccount := knd.Enum == models.DebtTxKind || knd.Enum == models.TransferTxKind
	hasGoal := knd.Enum == models.ExpenseTxKind

	if ftx.Category != nil {
		fd.CategoryID = &views.FDInput{Value: ftx.Category.ID, Error: valerr.Errors.MsgFor("CategoryID")}
	} else if !hasToAccount {
		fd.CategoryID = &views.FDInput{}
	}

	if ftx.Counterparty != nil {
		fd.CounterpartyID = &views.FDInput{Value: ftx.Counterparty.ID, Error: valerr.Errors.MsgFor("CounterpartyID")}
	} else if !hasToAccount {
		fd.CounterpartyID = &views.FDInput{}
	}

	if ftx.ToAccount != nil {
		fd.ToAccountIFD = &views.AccountIFD{
			Amount:    views.FDInput{Value: formatFloatIfNonZero(ftx.ToAmount), Error: valerr.Errors.MsgFor("ToAmount")},
			AccountID: views.FDInput{Value: ftx.ToAccount.ID, Error: valerr.Errors.MsgFor("ToAccountID")},
		}
	} else if hasToAccount {
		fd.ToAccountIFD = &views.AccountIFD{
			Amount:    views.FDInput{Error: valerr.Errors.MsgFor("ToAmount")},
			AccountID: views.FDInput{Error: valerr.Errors.MsgFor("ToAccountID")},
		}
	}

	if ftx.Goal != nil {
		fd.GoalID = &views.FDInput{Value: ftx.Goal.ID, Error: valerr.Errors.MsgFor("GoalID")}
	} else if hasGoal {
		fd.GoalID = &views.FDInput{Error: valerr.Errors.MsgFor("GoalID")}
	}

	return fd
}

func (h Handler) parseFrequencyFD(r *http.Request, err *errs.ValidationError) *views.FrequencyFD {
	return &views.FrequencyFD{
		IsFrequent: views.FDInput{Value: r.FormValue("is_frequent"), Error: err.Errors.MsgFor("IsFrequent")},
		FrequentBy: views.FDInput{Value: r.FormValue("frequent_by"), Error: err.Errors.MsgFor("FrequentBy")},
		Frequency:  views.FDInput{Value: r.FormValue("frequency"), Error: err.Errors.MsgFor("Frequency")},
		StopsOn:    views.FDInput{Value: r.FormValue("stops_on"), Error: err.Errors.MsgFor("StopsOn")},
	}
}
