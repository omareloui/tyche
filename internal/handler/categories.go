package handler

import (
	"net/http"
	"slices"
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

func (h Handler) Categories(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}

	cats, err := h.repo.GetCategories(r.Context())
	if err != nil {
		l.Error("fetching categories", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched categories")

	knds, err := h.repo.GetTransactionKinds(r.Context())
	if err != nil {
		return nil, err
	}
	knds = slicesutils.Filter(knds, func(k models.TransactionKind) bool { return k.Kind != models.TransferTxKind })

	return views.Categories(b, h.mapCategoryToFD(dfltCategory(knds), &errs.ValidationError{}), knds, cats), nil
}

func (h Handler) Category(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	cat, err := h.repo.GetCategory(r.Context(), id)
	if err != nil {
		l.Error("fetching category", zap.Error(err))
		return nil, err
	}
	l.Info("fetched category", zap.Any("category", cat))

	return views.Category(cat), nil
}

func (h Handler) CreateCategory(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	cat, err := h.mapFDToCategory(r)
	if err != nil {
		l.Error("parsing the form", zap.Error(err))
		return nil, err
	}
	cat.ID = idgen.New()
	cat.CreatedAt = time.Now()

	if err := h.sani.SanitizeStruct(cat); err != nil {
		l.Error("sanitizing the form", zap.Error(err))
		return nil, errs.ErrSanitizer
	}

	knds, err := h.repo.GetTransactionKinds(r.Context())
	if err != nil {
		return nil, err
	}
	knds = slicesutils.Filter(knds, func(k models.TransactionKind) bool { return k.Kind != models.TransferTxKind })

	if valerr := h.valid.Validate(cat); valerr != nil {
		l.Error("create category validation", zap.Error(valerr), zap.Any("errors", valerr.Errors))
		return views.CreateCategoryForm(h.mapCategoryToFD(cat, valerr), knds), nil
	}

	if err := h.repo.CreateCategory(r.Context(), cat); err != nil {
		l.Error("creating category", zap.Error(err))
		return nil, err
	}
	l.Info("created category")

	if err = renderToBody(w, r, views.CategoryOOB(cat)); err != nil {
		l.Error("rendering category oob", zap.Error(err))
		return nil, err
	}
	l.Debug("rendered category oob")

	return views.CreateCategoryForm(h.mapCategoryToFD(dfltCategory(knds), &errs.ValidationError{}), knds), nil
}

func (h Handler) GetEditCategory(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	cat, err := h.repo.GetCategory(r.Context(), id)
	if err != nil {
		l.Error("fetching category", zap.Error(err))
		return nil, err
	}
	l.Info("fetched category", zap.Any("category", cat))

	knds, err := h.repo.GetTransactionKinds(r.Context())
	if err != nil {
		return nil, err
	}
	knds = slicesutils.Filter(knds, func(k models.TransactionKind) bool { return k.Kind != models.TransferTxKind })

	return views.EditCategory(h.mapCategoryToFD(cat, &errs.ValidationError{}), id, knds), nil
}

func (h Handler) EditCategory(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	cat, err := h.mapFDToCategory(r)
	if err != nil {
		l.Error("parsing the form", zap.Error(err))
		return nil, err
	}
	cat.ID = id

	l.Debug("update category", zap.Any("category", cat))

	if err := h.repo.UpdateCategory(r.Context(), cat); err != nil {
		l.Error("update category", zap.Error(err))
		return nil, err
	}
	l.Info("updated category")

	return h.Category(w, r)
}

func (h Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())
	if err := h.repo.DeleteCategory(r.Context(), id); err != nil {
		l.Error("deleting category", zap.Error(err))
		return nil, err
	}
	l.Info("deleted category")

	return nil, nil
}

func (h Handler) mapCategoryToFD(cat *models.Category, err *errs.ValidationError) *views.CategoryFD {
	return &views.CategoryFD{
		Name:        views.FDInput{Value: cat.Name, Error: err.Errors.MsgFor("Name")},
		KindID:      views.FDInput{Value: cat.Kind.ID, Error: err.Errors.MsgFor("Kind.ID")},
		BudgetInEgp: views.FDInput{Value: formatFloatIfNonZero(cat.BudgetInEgp), Error: err.Errors.MsgFor("BudgetInEgp")},
	}
}

func (h Handler) mapFDToCategory(r *http.Request) (*models.Category, error) {
	budget, err := parseFloatIfExists(r.FormValue("budget_in_egp"))
	if err != nil {
		return nil, err
	}
	return &models.Category{
		Name:        r.FormValue("name"),
		Kind:        models.TransactionKind{ID: r.FormValue("kind_id")},
		BudgetInEgp: budget,
	}, nil
}

func dfltCategory(knds []models.TransactionKind) *models.Category {
	dfltKindIdx := slices.IndexFunc(knds, func(knd models.TransactionKind) bool { return knd.Kind == models.ExpenseTxKind })

	if dfltKindIdx == -1 {
		return &models.Category{
			Kind: models.TransactionKind{},
		}
	}

	return &models.Category{
		Kind: models.TransactionKind{ID: knds[dfltKindIdx].ID},
	}
}
