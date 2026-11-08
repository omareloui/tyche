package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

func (h Handler) RecurrenceForm(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	fd := h.parseRecurrenceFD(r, &errs.ValidationError{})

	if fd.Unit.Value == "" {
		fd.Unit.Value = fmt.Sprintf("%d", models.RecurrentUnitMonth)
		fd.Every.Value = "1"
	}

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

	accs, knds, cats, cps, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	// ?rtx= opens the recurrent transaction's details, e.g. when reloading.
	openID := r.URL.Query().Get(views.RecurrentTxDetailsParam)

	return views.RecurrentTxs(getClaims(r.Context()), balances, ftxs,
		newRecurrentTxFD(h, accs), accs, knds, cats, cps, openID), nil
}

// RecurrentTxDetails is the recurrent transaction in full, in a drawer: what
// it is, how often, and its upcoming and past transactions.
func (h Handler) RecurrentTxDetails(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
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
	txs, err := h.repo.GetRecurrentTxTransactions(r.Context(), id)
	if err != nil {
		l.Error("fetching recurrent tx transactions", zap.Error(err))
		return nil, err
	}

	d := &views.RecurrentTxDetailsData{RecurrentTx: rt, Def: &getClaims(r.Context()).DefaultCurrency}
	for _, tx := range txs {
		if tx.FulfilledAt.IsZero() {
			d.Upcoming = append(d.Upcoming, tx)
		} else {
			d.Past = append(d.Past, tx)
		}
	}
	slices.Reverse(d.Upcoming)

	return views.RecurrentTxDetails(d), nil
}

// fromRecurrentTxDetails is whether the request is an action of the details,
// answered with them.
func fromRecurrentTxDetails(r *http.Request) bool {
	return r.URL.Query().Get("from") == views.RecurrentTxDetailsFrom
}

// newRecurrentTxFD is the create form's data: starting today, monthly.
func newRecurrentTxFD(h Handler, accs []models.Account) *views.TransactionFD {
	tx := defaultTransaction(accs)
	tx.RecurrentTx = &models.RecurrentTx{Unit: models.RecurrentUnitMonth, Every: 1}
	return h.mapTransactionToFD(tx, new(errs.ValidationError))
}

// CreateRecurrentTx creates a recurrent transaction with its first
// transaction on its start date (fulfilled when not ahead), the next ones
// counted from it.
func (h Handler) CreateRecurrentTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	tx, err := h.mapFDToTransaction(r, true)
	if err != nil {
		return nil, err
	}
	tx.ID = idgen.New()

	if !tx.ScheduledTo.After(h.userToday(r.Context())) {
		tx.FulfilledAt = time.Now()
	}

	accs, knds, cats, cps, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	applyCounterpartyDefaultCategory(tx, cps)

	claims := getClaims(r.Context())

	fd, err := h.prepareTx(r, tx, accs, knds, cats, cps)
	if fd != nil {
		return views.CreateRecurrentTxForm(fd, accs, knds, cats, cps, &claims.DefaultCurrency), nil
	}
	if err != nil {
		return nil, err
	}

	rt := tx.RecurrentTx
	rt.ID = idgen.New()

	if err := rt.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating recurrent transaction", zap.Error(valerr), zap.Any("errors", valerr.Errors))
			return views.CreateRecurrentTxForm(h.mapTransactionToFD(tx, &valerr),
				accs, knds, cats, cps, &claims.DefaultCurrency), nil
		}
		l.Error("sanitizing recurrent transaction", zap.Error(err))
		return nil, err
	}

	if err := h.repo.CreateTransaction(r.Context(), tx, rt, nil); err != nil {
		l.Error("creating recurrent transaction", zap.Error(err))
		return nil, err
	}
	l.Info("created recurrent transaction", zap.String("id", rt.ID))
	drawerCreated(w)

	rts, err := h.repo.GetRecurrentTxs(r.Context())
	if err != nil {
		return nil, err
	}
	if err := renderToBody(w, r, views.RecurrentTxsListOOB(rts, &claims.DefaultCurrency)); err != nil {
		return nil, err
	}

	if !tx.FulfilledAt.IsZero() {
		if err := h.getAndRenderBalancesOOB(w, r); err != nil {
			return nil, err
		}
	}

	return views.CreateRecurrentTxForm(newRecurrentTxFD(h, accs), accs, knds, cats, cps, &claims.DefaultCurrency), nil
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

	return views.RecurrentTx(rt), nil
}

func (h Handler) GetEditRecurrentTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	accs, knds, cats, cps, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	rt, err := h.liveRecurrentTx(r)
	if err != nil {
		return nil, err
	}
	l.Info("fetched recurrent tx")

	claims := getClaims(r.Context())

	return views.EditRecurrentTx(rt,
		h.mapRecurrentTxToFD(rt, &errs.ValidationError{}, knds),
		accs, knds, cats, cps, &claims.DefaultCurrency), nil
}

func (h Handler) EditRecurrentTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	old, err := h.liveRecurrentTx(r)
	if err != nil {
		return nil, err
	}

	tx, err := h.mapFDToTransaction(r, true)
	if err != nil {
		l.Error("parsing formdata", zap.Error(err))
		return nil, err
	}

	rt := tx.RecurrentTx
	rt.ID = id
	rt.StartsAt = old.StartsAt
	rt.PausedAt = old.PausedAt
	models.NormalizeTxItems(tx.Items, &old.Account.Currency)
	rt.Items = models.SortTxItems(tx.Items)

	// The schedule is counted again from the next day picked, when it or the
	// frequency changed.
	nextOn, err := parseDateOnlyIfExists(r.FormValue("next_on"))
	if err != nil {
		return nil, err
	}
	rt.Anchor = old.Anchor
	if !nextOn.IsZero() && (!nextOn.Equal(nextOccurrence(old)) || rt.Unit != old.Unit || rt.Every != old.Every) {
		rt.Anchor = nextOn
	} else if rt.Unit != old.Unit || rt.Every != old.Every {
		rt.Anchor = nextOccurrence(old)
	}

	accs, knds, cats, cps, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	if err := rt.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating recurrent transaction", zap.Error(valerr), zap.Any("errors", valerr.Errors))
			claims := getClaims(r.Context())
			fd := h.mapRecurrentTxToFD(rt, &valerr, knds)
			fd.NextOn = form.FDInput{Value: r.FormValue("next_on"), Error: valerr.Errors.MsgFor("Anchor")}
			return views.EditRecurrentTxForm(rt, fd, accs, knds, cats, cps, &claims.DefaultCurrency), nil
		}
		l.Error("sanitizing recurrent transaction", zap.Error(err))
		return nil, err
	}

	if err := h.repo.UpdateRecurrentTx(r.Context(), old, rt); err != nil {
		l.Error("updating recurrent tx", zap.Error(err))
		return nil, err
	}
	l.Info("updated recurrent tx", zap.String("id", rt.ID))
	drawerSaved(w, "#"+views.RecurrentTxID(id), "outerHTML")

	rt, err = h.repo.GetRecurrentTx(r.Context(), id)
	if err != nil {
		l.Error("fetching recurrent tx", zap.Error(err))
		return nil, err
	}

	if err := h.renderRecurrentTxsSummaryOOB(w, r); err != nil {
		return nil, err
	}

	return views.RecurrentTx(rt), nil
}

// nextOccurrence is the day of rt's next transaction: its first scheduled
// one, else the next it writes.
func nextOccurrence(rt *models.RecurrentTx) time.Time {
	if len(rt.Upcoming) > 0 {
		return dateOnly(rt.Upcoming[0])
	}
	return rt.NextOn()
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// liveRecurrentTx is the recurrent tx of the path's id, refused when it pays
// a gam'yaa's installments: its schedule is only changed through its
// transactions.
func (h Handler) liveRecurrentTx(r *http.Request) (*models.RecurrentTx, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	rt, err := h.repo.GetRecurrentTx(r.Context(), id)
	if err != nil {
		logger.FromCtx(r.Context()).Error("fetching recurrent tx", zap.Error(err), zap.String("id", id))
		return nil, err
	}
	if rt.IsGamyaa() {
		return nil, resperr.New(http.StatusForbidden, "A gam'yaa's installments change through its transactions.")
	}
	return rt, nil
}

// SkipRecurrentOccurrence deletes one of its scheduled transactions
// (DELETE /recurrent-tx/{id}/occurrences/{txID}), never written again,
// answering its details.
func (h Handler) SkipRecurrentOccurrence(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	rt, err := h.liveRecurrentTx(r)
	if err != nil {
		return nil, err
	}
	tx, err := h.repo.GetTransaction(r.Context(), r.PathValue("txID"))
	if err != nil {
		return nil, err
	}
	if tx.RecurrentTxID != rt.ID || !tx.FulfilledAt.IsZero() {
		return nil, resperr.New(http.StatusNotFound, "That isn't one of its scheduled transactions.")
	}
	if err := h.repo.DeleteTransaction(r.Context(), tx); err != nil {
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("skipped an occurrence", zap.String("recurrent_id", rt.ID), zap.String("id", tx.ID))

	if err := h.renderRecurrentTxsSummaryOOB(w, r); err != nil {
		return nil, err
	}
	if rt, err = h.repo.GetRecurrentTx(r.Context(), rt.ID); err != nil {
		return nil, err
	}
	if err := renderToBody(w, r, views.RecurrentTxOOB(rt)); err != nil {
		return nil, err
	}
	return h.RecurrentTxDetails(w, r)
}

// PauseRecurrentTx stops scheduling it (PUT /recurrent-tx/{id}/pause),
// ResumeRecurrentTx starts again from today (PUT /recurrent-tx/{id}/resume).
func (h Handler) PauseRecurrentTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return h.setRecurrentTxPaused(w, r, true)
}

func (h Handler) ResumeRecurrentTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return h.setRecurrentTxPaused(w, r, false)
}

func (h Handler) setRecurrentTxPaused(w http.ResponseWriter, r *http.Request, paused bool) (templ.Component, error) {
	rt, err := h.liveRecurrentTx(r)
	if err != nil {
		return nil, err
	}
	if paused {
		err = h.repo.PauseRecurrentTx(r.Context(), rt)
	} else {
		err = h.repo.ResumeRecurrentTx(r.Context(), rt)
	}
	if err != nil {
		logger.FromCtx(r.Context()).Error("pausing recurrent tx", zap.Bool("paused", paused), zap.Error(err))
		return nil, err
	}

	if err := h.renderRecurrentTxsSummaryOOB(w, r); err != nil {
		return nil, err
	}
	if fromRecurrentTxDetails(r) {
		rt, err := h.repo.GetRecurrentTx(r.Context(), rt.ID)
		if err != nil {
			return nil, err
		}
		if err := renderToBody(w, r, views.RecurrentTxOOB(rt)); err != nil {
			return nil, err
		}
		return h.RecurrentTxDetails(w, r)
	}
	return h.RecurrentTx(w, r)
}

func (h Handler) DeleteRecurrentTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	rt, err := h.liveRecurrentTx(r)
	if err != nil {
		return nil, err
	}
	id := rt.ID

	l := logger.FromCtx(r.Context())

	if err := h.repo.DeleteRecurrentTx(r.Context(), id); err != nil {
		l.Error("deleting recurrent transaction", zap.Error(err))
		return nil, err
	}
	l.Info("deleted recurrent transaction")
	if fromRecurrentTxDetails(r) {
		addHXTrigger(w, drawerSavedEvent)
	}

	if err := h.renderRecurrentTxsSummaryOOB(w, r); err != nil {
		return nil, err
	}

	return nil, nil
}

func (h Handler) renderRecurrentTxsSummaryOOB(w http.ResponseWriter, r *http.Request) error {
	rts, err := h.repo.GetRecurrentTxs(r.Context())
	if err != nil {
		logger.FromCtx(r.Context()).Error("fetching recurrent txs for summary", zap.Error(err))
		return err
	}
	claims := getClaims(r.Context())
	return renderToBody(w, r, views.RecurrentTxsSummaryOOB(rts, &claims.DefaultCurrency))
}

func (h Handler) mapRecurrentTxToFD(rt *models.RecurrentTx, valerr *errs.ValidationError, knds []models.TxKind) *views.RecurrentTxFD {
	fd := &views.RecurrentTxFD{
		RecurrenceFD: views.RecurrenceFD{
			IsRecurrent: form.FDInput{Value: "on"},
			Unit:        form.FDInput{Value: formatIntIfNonZero(int(rt.Unit)), Error: valerr.Errors.MsgFor("Unit")},
			Every:       form.FDInput{Value: formatIntIfNonZero(int(rt.Every)), Error: valerr.Errors.MsgFor("Every")},
			StopsOn:     form.FDInput{Value: formatDateOnlyIfNonZero(rt.StopsAt), Error: valerr.Errors.MsgFor("StopsAt")},
			AutoFulfill: form.FDInput{Value: onIf(rt.AutoFulfill)},
			NextOn:      form.FDInput{Value: formatDateOnlyIfNonZero(nextOccurrence(rt)), Error: valerr.Errors.MsgFor("Anchor")},
			WithNextOn:  true,
		},
		TransactionAmountsAndKindFD: views.TransactionAmountsAndKindFD{
			AccountIFD: amountIFD(rt.Amount, &rt.Account, form.FDInput{Value: rt.Account.ID, Error: valerr.Errors.MsgFor("AccountID")},
				valerr.Errors.MsgFor("Amount")),
			Kind: form.FDInput{Value: rt.Kind.String(), Error: valerr.Errors.MsgFor("Kind")},
		},
		Description: form.FDInput{Value: rt.Description, Error: valerr.Errors.MsgFor("Description")},
		Items:       txItemsToFD(rt.Items, valerr),
	}

	hasToAccount := rt.Kind == models.DebtTxKind || rt.Kind == models.TransferTxKind

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
		toIFD := amountIFD(rt.ToAmount, rt.ToAccount, form.FDInput{Value: rt.ToAccount.ID, Error: valerr.Errors.MsgFor("ToAccountID")},
			valerr.Errors.MsgFor("ToAmount"))
		fd.ToAccountIFD = &toIFD
	} else if hasToAccount {
		fd.ToAccountIFD = &form.AccountIFD{
			Amount:    form.FDInput{Error: valerr.Errors.MsgFor("ToAmount")},
			AccountID: form.FDInput{Error: valerr.Errors.MsgFor("ToAccountID")},
		}
	}

	return fd
}

func (h Handler) parseRecurrenceFD(r *http.Request, err *errs.ValidationError) *views.RecurrenceFD {
	return &views.RecurrenceFD{
		IsRecurrent: form.FDInput{Value: r.FormValue("is_recurrent"), Error: err.Errors.MsgFor("IsRecurrent")},
		Every:       form.FDInput{Value: r.FormValue("every"), Error: err.Errors.MsgFor("RecurrentBy")},
		Unit:        form.FDInput{Value: r.FormValue("unit"), Error: err.Errors.MsgFor("Recurrence")},
		StopsOn:     form.FDInput{Value: r.FormValue("stops_on"), Error: err.Errors.MsgFor("StopsAt")},
		AutoFulfill: form.FDInput{Value: r.FormValue("auto_fulfill")},
	}
}

// DailyUpkeep runs the daily upkeep of the tenant in ctx (writing the
// recurrent transactions' occurrences due, ...), once a day
// (middleware.DailyUpkeep).
func (h *Handler) DailyUpkeep(ctx context.Context, tenantID string) {
	h.repo.RunDailyUpkeep(ctx, tenantID)
}
