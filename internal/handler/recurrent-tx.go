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
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

const generateNextScheduledTransactionsCount = 5

func (h Handler) RecurrenceForm(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	id := r.PathValue("id")
	fd := h.parseRecurrenceFD(r, &errs.ValidationError{})

	if fd.Unit.Value == "" {
		fd.Unit.Value = fmt.Sprintf("%d", models.RecurrentUnitMonth)
		fd.Every.Value = "1"
	}

	l.Info("responding with", zap.Any("formdata", fd))

	return views.RecurrenceFormBody(id, fd), nil
}

func (h Handler) RecurrentTxs(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	balances, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched balances")

	ftxs, err := h.repo.GetRecurrentTxs(r.Context())
	if err != nil {
		return nil, err
	}
	l.Info("fetched recurrent transactions")

	return views.RecurrentTxs(getClaims(r.Context()), balances, ftxs), nil
}

func (h Handler) RecurrentTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	rt, err := h.repo.GetRecurrentTx(r.Context(), id)
	if err != nil {
		l.Error("fetching recurrent tx", zap.Error(err))
		return nil, err
	}
	l.Info("fetched recurrent tx", zap.Any("recurrent tx", rt))

	return views.RecurrentTx(rt), nil
}

func (h Handler) GetEditRecurrentTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	accs, knds, cats, cps, gls, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	rt, err := h.repo.GetRecurrentTx(r.Context(), id)
	if err != nil {
		l.Error("fetching recurrent tx", zap.Error(err))
		return nil, err
	}
	l.Info("fetched recurrent tx")

	claims := getClaims(r.Context())

	return views.EditRecurrentTx(rt,
		h.mapRecurrentTxToFD(rt, &errs.ValidationError{}, knds),
		accs, knds, cats, cps, gls, &claims.DefaultCurrency), nil
}

func (h Handler) EditRecurrentTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())
	tx, err := h.mapFDToTransaction(r, true)
	if err != nil {
		l.Error("parsing formdata", zap.Error(err))
		return nil, err
	}

	rt := tx.RecurrentTx
	rt.ID = id
	rt.LastTransactionScheduledTo = time.Now()

	accs, knds, cats, cps, gls, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	oldFtx, err := h.repo.GetRecurrentTx(r.Context(), id)
	if err != nil {
		l.Error("getting old recurrent tx", zap.String("id", id), zap.Error(err))
		return nil, err
	}

	rt.StartsAt = oldFtx.StartsAt

	if err := rt.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating recurrent transaction", zap.Error(valerr), zap.Any("errors", valerr.Errors))
			claims := getClaims(r.Context())
			return views.EditRecurrentTx(rt, h.mapRecurrentTxToFD(rt, &valerr, knds),
				accs, knds, cats, cps, gls, &claims.DefaultCurrency), nil
		}
		l.Error("sanitizing recurrent transaction", zap.Error(err))
		return nil, err
	}

	scheduled := generateNextScheduledTransactions(rt, generateNextScheduledTransactionsCount)

	if err := h.repo.UpdateRecurrentTx(r.Context(), rt, scheduled); err != nil {
		l.Error("updating recurrent tx", zap.Error(err))
		return nil, err
	}
	l.Info("updated recurrent tx", zap.Any("recurrent_tx", rt))

	rt, err = h.repo.GetRecurrentTx(r.Context(), id)
	if err != nil {
		l.Error("fetching recurrent tx", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched recurrent tx", zap.Any("recurrent_tx", rt))

	return views.RecurrentTx(rt), nil
}

func (h Handler) GenerateTransactions(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	rt, err := h.repo.GetRecurrentTx(r.Context(), id)
	if err != nil {
		l.Error("fetching recurrent tx", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched recurrent tx", zap.Any("recurrent tx", rt))

	scheduled := generateNextScheduledTransactions(rt, generateNextScheduledTransactionsCount)

	if err := h.repo.GenerateScheduledTransactions(r.Context(), rt, scheduled); err != nil {
		return nil, err
	}

	return views.RecurrentTx(rt), nil
}

func (h Handler) DeleteRecurrentTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	if err := h.repo.DeleteRecurrentTx(r.Context(), id); err != nil {
		l.Error("deleting recurrent transaction", zap.Error(err))
		return nil, err
	}
	l.Info("deleted recurrent transaction")

	return nil, nil
}

func generateNextScheduledTransactions(rt *models.RecurrentTx, count int) []models.Transaction {
	txs := []models.Transaction{}
	now := time.Now()

	for range count {
		var nextOn time.Time

		switch rt.Unit {
		case models.RecurrentUnitDay:
			nextOn = rt.LastTransactionScheduledTo.AddDate(0, 0, int(rt.Every))
		case models.RecurrentUnitMonth:
			nextOn = rt.LastTransactionScheduledTo.AddDate(0, int(rt.Every), 0)
			if d := nextOn.Day(); d != rt.LastTransactionScheduledTo.Day() {
				nextOn = nextOn.AddDate(0, 0, -d)
			}
		case models.RecurrentUnitYear:
			nextOn = rt.LastTransactionScheduledTo.AddDate(int(rt.Every), 0, 0)
			if d := nextOn.Day(); d != rt.LastTransactionScheduledTo.Day() {
				nextOn = nextOn.AddDate(0, 0, -d)
			}
		}

		nextOn = nextOn.Truncate(24 * time.Hour)

		if !rt.StopsAt.IsZero() && nextOn.After(rt.StopsAt) {
			break
		}

		txs = append(txs, models.Transaction{
			ID:           idgen.New(),
			Account:      rt.Account,
			Amount:       rt.Amount,
			Kind:         rt.Kind,
			Category:     rt.Category,
			Counterparty: rt.Counterparty,
			ToAccount:    rt.ToAccount,
			ToAmount:     rt.ToAmount,
			Description:  rt.Description,
			RecurrentTx:  &models.RecurrentTx{ID: rt.ID},
			ScheduledTo:  nextOn,
			CreatedAt:    now,
		})
		rt.LastTransactionScheduledTo = nextOn
	}
	return txs
}

func (h Handler) mapRecurrentTxToFD(rt *models.RecurrentTx, valerr *errs.ValidationError, knds []models.TxKind) *views.RecurrentTxFD {
	fd := &views.RecurrentTxFD{
		RecurrenceFD: views.RecurrenceFD{
			IsRecurrent: form.FDInput{Value: "on"},
			Unit:        form.FDInput{Value: formatIntIfNonZero(int(rt.Unit)), Error: valerr.Errors.MsgFor("Unit")},
			Every:       form.FDInput{Value: formatIntIfNonZero(int(rt.Every)), Error: valerr.Errors.MsgFor("Every")},
			StopsOn:     form.FDInput{Value: formatDateOnlyIfNonZero(rt.StopsAt), Error: valerr.Errors.MsgFor("StopsOn")},
		},
		TransactionAmountsAndKindFD: views.TransactionAmountsAndKindFD{
			AccountIFD: form.AccountIFD{
				AccountID: form.FDInput{Value: rt.Account.ID, Error: valerr.Errors.MsgFor("AccountID")},
				Amount:    form.FDInput{Value: formatFloatIfNonZero(rt.Amount), Error: valerr.Errors.MsgFor("Amount")},
			},
			Kind: form.FDInput{Value: rt.Kind.String(), Error: valerr.Errors.MsgFor("Kind")},
		},
		Description: form.FDInput{Value: rt.Description, Error: valerr.Errors.MsgFor("Description")},
	}

	hasToAccount := rt.Kind == models.DebtTxKind || rt.Kind == models.TransferTxKind
	hasGoal := rt.Kind == models.ExpenseTxKind

	if rt.Category != nil {
		fd.CategoryID = &form.FDInput{Value: rt.Category.ID, Error: valerr.Errors.MsgFor("CategoryID")}
	} else if !hasToAccount {
		fd.CategoryID = &form.FDInput{}
	}

	if rt.Counterparty != nil {
		fd.CounterpartyID = &form.FDInput{Value: rt.Counterparty.ID, Error: valerr.Errors.MsgFor("CounterpartyID")}
	} else if !hasToAccount {
		fd.CounterpartyID = &form.FDInput{}
	}

	if rt.ToAccount != nil {
		fd.ToAccountIFD = &form.AccountIFD{
			Amount:    form.FDInput{Value: formatFloatIfNonZero(rt.ToAmount), Error: valerr.Errors.MsgFor("ToAmount")},
			AccountID: form.FDInput{Value: rt.ToAccount.ID, Error: valerr.Errors.MsgFor("ToAccountID")},
		}
	} else if hasToAccount {
		fd.ToAccountIFD = &form.AccountIFD{
			Amount:    form.FDInput{Error: valerr.Errors.MsgFor("ToAmount")},
			AccountID: form.FDInput{Error: valerr.Errors.MsgFor("ToAccountID")},
		}
	}

	if rt.Goal != nil {
		fd.GoalID = &form.FDInput{Value: rt.Goal.ID, Error: valerr.Errors.MsgFor("GoalID")}
	} else if hasGoal {
		fd.GoalID = &form.FDInput{Error: valerr.Errors.MsgFor("GoalID")}
	}

	return fd
}

func (h Handler) parseRecurrenceFD(r *http.Request, err *errs.ValidationError) *views.RecurrenceFD {
	return &views.RecurrenceFD{
		IsRecurrent: form.FDInput{Value: r.FormValue("is_recurrent"), Error: err.Errors.MsgFor("IsRecurrent")},
		Every:       form.FDInput{Value: r.FormValue("every"), Error: err.Errors.MsgFor("RecurrentBy")},
		Unit:        form.FDInput{Value: r.FormValue("unit"), Error: err.Errors.MsgFor("Recurrence")},
		StopsOn:     form.FDInput{Value: r.FormValue("stops_on"), Error: err.Errors.MsgFor("StopsOn")},
	}
}
