package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-planner/internal/errs"
	"github.com/omareloui/financial-planner/internal/idgen"
	"github.com/omareloui/financial-planner/internal/logger"
	"github.com/omareloui/financial-planner/internal/models"
	"github.com/omareloui/financial-planner/internal/slicesutils"
	"github.com/omareloui/financial-planner/web/views"
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

	return views.Goals(b, its, h.mapGlToFD(dfltGoal(), &errs.ValidationError{}), gls), nil
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
	gl.EgpSumTotal = gl.CalculateEgpSubtotal()

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
