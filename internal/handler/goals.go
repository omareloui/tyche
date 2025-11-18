package handler

import (
	"errors"
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

func (h Handler) Goals(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched balances")

	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched balances")

	gls, err := h.repo.GetGoals(r.Context())
	if err != nil {
		l.Error("fetching goals", zap.Error(err))
		return nil, err
	}
	l.Info("fetched goals", zap.Int("count", len(gls)))

	claims := getClaims(r.Context())
	return views.Goals(claims, b, h.mapGlToFD(dfltGoal(&claims.DefaultCurrency), &errs.ValidationError{}), gls, curs), nil
}

func (h Handler) Goal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	gl, err := h.repo.GetGoal(r.Context(), id)
	if err != nil {
		l.Error("fetching goal", zap.Error(err))
		return nil, err
	}
	l.Info("fetched goal")

	return views.Goal(gl), nil
}

func (h Handler) CreateGoal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	gl, err := h.mapFDToGoal(r)
	if err != nil {
		l.Error("parsing form to goal", zap.Error(err))
		return nil, err
	}
	l.Debug("parsed form to goal", zap.Any("goal", gl))

	gl.ID = idgen.New()
	gl.CreatedAt = time.Now()

	curs, err := h.repo.GetAllCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched balances")

	shownCurs := slicesutils.Filter(curs, func(cur models.Currency) bool { return cur.IsShown })

	claims := getClaims(r.Context())

	if err := gl.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("create goal validation", zap.Error(valerr), zap.Any("errors", valerr.Errors))
			return views.CreateGoalForm(h.mapGlToFD(gl, &valerr), shownCurs, &claims.DefaultCurrency), nil
		}
		l.Error("sanitizing goal", zap.Error(err))
		return nil, err
	}

	if err = h.repo.CreateGoal(r.Context(), gl); err != nil {
		l.Error("creating goal", zap.Error(err))
		return nil, err
	}
	l.Info("created goal", zap.Any("goal", gl))

	if err = renderToBody(w, r, views.GoalOOB(gl)); err != nil {
		l.Error("rendering OOB to body", zap.Error(err))
		return nil, err
	}
	l.Debug("rendered goal")

	return views.CreateGoalForm(h.mapGlToFD(dfltGoal(&claims.DefaultCurrency), &errs.ValidationError{}), shownCurs, &claims.DefaultCurrency), nil
}

func (h Handler) GetEditGoal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched balances")

	gl, err := h.repo.GetGoal(r.Context(), id)
	if err != nil {
		l.Error("fetching goal", zap.Error(err))
		return nil, err
	}
	l.Info("fetched goal")

	claims := getClaims(r.Context())
	return views.EditGoal(id, h.mapGlToFD(gl, &errs.ValidationError{}), curs, &claims.DefaultCurrency), nil
}

func (h Handler) GetGoalProgressForm(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	gl, err := h.repo.GetGoal(r.Context(), id)
	if err != nil {
		return nil, err
	}

	accs, knds, _, _, _, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	claims := getClaims(r.Context())

	fd := h.mapTransactionToFD(defaultTransaction(accs, knds), &errs.ValidationError{}, knds)
	return views.GoalProgressForm(gl, fd, accs, knds, &claims.DefaultCurrency), nil
}

func (h Handler) CreateGoalProgress(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	gl, err := h.repo.GetGoal(r.Context(), id)
	if err != nil {
		l.Error("getting goal", zap.String("id", id), zap.Error(err))
		return nil, err
	}

	tx, err := h.mapFDToTransaction(r, r.FormValue("is_recurrent") != "")
	if err != nil {
		l.Error("mapping from data to transaction for goal", zap.Any("tx", tx), zap.Any("goal", gl), zap.Error(err))
		return nil, err
	}
	tx.Goal = gl

	accs, knds, cats, cps, gls, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	knd := slicesutils.Find(knds, func(k models.TransactionKind) bool { return k.Enum == models.ExpenseTxKind })
	if knd != nil {
		tx.Kind = *knd
	}

	fd, err := h.prepareCreatingTx(r, tx, accs, knds, cats, cps, gls)
	if fd != nil {
		claims := getClaims(r.Context())
		return views.GoalProgressForm(gl, fd, accs, knds, &claims.DefaultCurrency), nil
	}
	if err != nil {
		return nil, err
	}

	if err = h.repo.CreateTransaction(r.Context(), tx, nil, nil); err != nil {
		l.Error("creating transaction for goal", zap.Any("tx", tx), zap.Any("goal", gl), zap.Error(err))
		return nil, err
	}

	if !tx.FulfilledAt.IsZero() {
		if err := h.getAndRenderBalancesOOB(w, r); err != nil {
			l.Error("getting and rendering balance", zap.Any("tx", tx), zap.Any("goal", gl), zap.Error(err))
			return nil, err
		}
	}

	gl.Transactions = append(gl.Transactions, *tx)

	return views.Goal(gl), nil
}

func (h Handler) EditGoal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	gl, err := h.mapFDToGoal(r)
	if err != nil {
		l.Error("parsing form to goal", zap.Error(err))
		return nil, err
	}
	l.Debug("parsed form to goal", zap.Any("goal", gl))

	gl.ID = id

	curs, err := h.repo.GetAllCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies for updating goal", zap.Error(err))
		return nil, err
	}
	shownCurs := slicesutils.Filter(curs, func(cur models.Currency) bool { return cur.IsShown })

	gl.Currency = *slicesutils.Find(curs, func(cur models.Currency) bool { return cur.ID == gl.Currency.ID })

	if err := gl.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("update goal validation", zap.Error(valerr), zap.Any("errors", valerr.Errors))
			claims := getClaims(r.Context())
			fd := h.mapGlToFD(gl, &valerr)
			return views.EditGoal(id, fd, shownCurs, &claims.DefaultCurrency), nil
		}
		l.Error("sanitizing goal", zap.Error(err))
		return nil, err
	}

	if err = h.repo.UpdateGoal(r.Context(), gl); err != nil {
		l.Error("updating goal", zap.Error(err))
		return nil, err
	}
	l.Info("updated goal", zap.Any("goal", gl))

	return views.Goal(gl), nil
}

func (h Handler) FulfillGoal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	if err := h.repo.FulfillGoal(r.Context(), id); err != nil {
		return nil, err
	}
	return h.Goal(w, r)
}

func (h Handler) UnfulfillGoal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	if err := h.repo.UnfulfillGoal(r.Context(), id); err != nil {
		return nil, err
	}
	return h.Goal(w, r)
}

func (h Handler) DeleteGoal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	l := logger.FromCtx(r.Context())

	err := h.repo.DeleteGoal(r.Context(), id)
	if err != nil {
		l.Error("deleting goal", zap.Error(err), zap.String("id", id))
		return nil, err
	}
	l.Info("deleted goal", zap.String("id", id))

	return nil, nil
}

func (h Handler) mapGlToFD(gl *models.Goal, err *errs.ValidationError) *views.GoalFD {
	return &views.GoalFD{
		Name: views.FDInput{Value: gl.Name, Error: err.Errors.MsgFor("Name")},
		AmountFD: &views.AmountFD{
			Amount:     views.FDInput{Value: formatFloatIfNonZero(gl.Price), Error: err.Errors.MsgFor("Price")},
			CurrencyID: views.FDInput{Value: gl.Currency.ID, Error: err.Errors.MsgFor("Currency.ID")},
		},
	}
}

func (h Handler) mapFDToGoal(r *http.Request) (*models.Goal, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}

	amt, err := parseFloatIfExists(r.FormValue("amount"))
	if err != nil {
		return nil, err
	}

	return &models.Goal{
		Name:     r.FormValue("name"),
		Price:    amt,
		Currency: models.Currency{ID: r.FormValue("currency_id")},
	}, nil
}

func dfltGoal(dfltCur *models.Currency) *models.Goal {
	return &models.Goal{Currency: *dfltCur}
}
