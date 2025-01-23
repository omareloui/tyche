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

	its, err := h.repo.GetItems(r.Context())
	if err != nil {
		l.Error("fetching items", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched items")

	gls, err := h.repo.GetGoals(r.Context())
	if err != nil {
		l.Error("fetching goals", zap.Error(err))
		return nil, err
	}
	l.Info("fetched goals", zap.Int("count", len(gls)))

	return views.Goals(getClaims(r), b, its, h.mapGlToFD(dfltGoal(), &errs.ValidationError{}), gls), nil
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

	its, err := h.repo.GetItems(r.Context())
	if err != nil {
		l.Error("fetching items", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched items")

	h.populateItemsToGoal(its, gl)
	gl.ID = idgen.New()
	gl.CreatedAt = time.Now()
	gl.EgpSubtotal = gl.CalculateEgpSubtotal()

	if err = h.sani.SanitizeStruct(gl); err != nil {
		l.Error("sanitizing goal", zap.Error(err))
		return nil, err
	}

	if valerr := h.valid.Validate(gl); valerr != nil {
		l.Error("validating goal", zap.Any("errors", valerr.Errors))
		fd := h.mapGlToFD(gl, valerr)
		return views.CreateGoalForm(fd, its), nil
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

	return views.CreateGoalForm(h.mapGlToFD(dfltGoal(), &errs.ValidationError{}), its), nil
}

func (h Handler) GetEditGoal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
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

	its, err := h.repo.GetItems(r.Context())
	if err != nil {
		l.Error("fetching items", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched items")

	return views.EditGoal(id, h.mapGlToFD(gl, &errs.ValidationError{}), its), nil
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

	accs, knds, _, _, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	fd := h.mapTransactionToFD(defaultTransaction(accs, knds), &errs.ValidationError{}, knds)
	return views.GoalProgressForm(gl, fd, accs, knds), nil
}

func (h Handler) CreateGoalProgress(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())
	gl, err := h.repo.GetGoal(r.Context(), id)
	if err != nil {
		return nil, err
	}

	tx, err := h.mapFDToTransaction(r)
	if err != nil {
		return nil, err
	}

	accs, knds, cats, cps, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}
	knd := slicesutils.Find(knds, func(k models.TransactionKind) bool { return k.Enum == models.ExpenseTxKind })
	if knd != nil {
		tx.Kind = *knd
	}
	fd, err := h.prepareCreatingTx(r, tx, accs, knds, cats, cps)
	if fd != nil {
		return views.GoalProgressForm(gl, fd, accs, knds), nil
	}
	if err != nil {
		return nil, err
	}

	if err := h.repo.CreateGoalProgress(r.Context(), id, tx); err != nil {
		l.Error("creating transaction", zap.Error(err))
		return nil, err
	}
	l.Info("created transaction")

	if !tx.FulfilledAt.IsZero() {
		if err := h.getAndRenderBalancesOOB(w, r); err != nil {
			return nil, err
		}
	}

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

	its, err := h.repo.GetItems(r.Context())
	if err != nil {
		l.Error("fetching items", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched items")

	h.populateItemsToGoal(its, gl)
	gl.ID = id
	gl.EgpSubtotal = gl.CalculateEgpSubtotal()

	if err = h.sani.SanitizeStruct(gl); err != nil {
		l.Error("sanitizing goal", zap.Error(err))
		return nil, err
	}

	if valerr := h.valid.Validate(gl); valerr != nil {
		l.Error("validating goal", zap.Any("errors", valerr.Errors))
		fd := h.mapGlToFD(gl, valerr)
		return views.EditGoal(id, fd, its), nil
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
	return nil, nil
}

func (h Handler) UnfulfillGoal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	if err := h.repo.UnfulfillGoal(r.Context(), id); err != nil {
		return nil, err
	}
	return nil, nil
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

func (h Handler) populateItemsToGoal(its []models.Item, gl *models.Goal) {
	for i, glIt := range gl.Items {
		gl.Items[i] = *slicesutils.Find(its, func(it models.Item) bool { return glIt.ID == it.ID })
	}
}

func (h Handler) mapGlToFD(gl *models.Goal, err *errs.ValidationError) *views.GoalFD {
	itsFd := make([]views.FDInput, len(gl.Items))
	for i := range gl.Items {
		itsFd[i] = views.FDInput{Value: gl.Items[i].ID, Error: err.Errors.MsgFor(fmt.Sprintf("Items[%d]", i))}
	}
	return &views.GoalFD{
		Title: views.FDInput{Value: gl.Title, Error: err.Errors.MsgFor("Title")},
		Items: itsFd,
	}
}

func (h Handler) mapFDToGoal(r *http.Request) (*models.Goal, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	fdIts := r.Form["item_id"]
	its := make([]models.Item, len(fdIts))
	for i, fdIt := range fdIts {
		its[i] = models.Item{ID: fdIt}
	}
	return &models.Goal{
		Title: r.FormValue("title"),
		Items: its,
	}, nil
}

func dfltGoal() *models.Goal {
	return &models.Goal{
		Items: []models.Item{},
	}
}
