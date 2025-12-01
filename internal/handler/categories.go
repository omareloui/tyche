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
	"github.com/omareloui/tyche/web/views/form"
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

	knds := getAllowedKindsForCategories()

	currs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}

	claims := getClaims(r.Context())
	return views.Categories(claims, b, h.mapCategoryToFD(dfltCategory(&claims.DefaultCurrency), &errs.ValidationError{}), knds, cats, currs), nil
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

	if cat.Color == "" {
		cat.Color = models.GetRandomCategoryColor()
	}

	knds := getAllowedKindsForCategories()

	currs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}

	claims := getClaims(r.Context())

	if err := cat.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("create category validation", zap.Error(valerr), zap.Any("errors", valerr.Errors))
			return views.CreateCategoryForm(h.mapCategoryToFD(cat, &valerr), knds, currs, &claims.DefaultCurrency), nil
		}
		l.Error("sanitizing category", zap.Error(err))
		return nil, err
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

	return views.CreateCategoryForm(h.mapCategoryToFD(dfltCategory(&claims.DefaultCurrency), &errs.ValidationError{}), knds, currs, &claims.DefaultCurrency), nil
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

	knds := getAllowedKindsForCategories()

	currs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}

	claims := getClaims(r.Context())

	fd := h.mapCategoryToFD(cat, &errs.ValidationError{})
	if fd.BudgetAmountFD.CurrencyID.Value == "" {
		fd.BudgetAmountFD.CurrencyID.Value = claims.DefaultCurrency.ID
	}

	return views.EditCategory(fd, id, knds, currs, &claims.DefaultCurrency), nil
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

	knds := getAllowedKindsForCategories()
	currs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}

	claims := getClaims(r.Context())

	if err := cat.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("update category validation", zap.Error(valerr), zap.Any("errors", valerr.Errors))
			return views.EditCategory(h.mapCategoryToFD(cat, &valerr), id, knds, currs, &claims.DefaultCurrency), nil
		}
		l.Error("sanitizing category", zap.Error(err))
		return nil, err
	}

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
	var budgetCurrId string
	if cat.BudgetCurrency != nil {
		budgetCurrId = cat.BudgetCurrency.ID
	}
	return &views.CategoryFD{
		Name:  form.FDInput{Value: cat.Name, Error: err.Errors.MsgFor("Name")},
		Color: form.FDInput{Value: cat.Color, Error: err.Errors.MsgFor("Color")},
		// KindID: form.FDInput{Value: cat.Kind.ID, Error: err.Errors.MsgFor("Kind.ID")},
		BudgetAmountFD: &form.AmountFD{
			Amount:     form.FDInput{Value: formatFloatIfNonZero(cat.BudgetAmount), Error: err.Errors.MsgFor("BudgetAmount")},
			CurrencyID: form.FDInput{Value: budgetCurrId, Error: err.Errors.MsgFor("BudgetCurrency.ID")},
		},
	}
}

func (h Handler) mapFDToCategory(r *http.Request) (*models.Category, error) {
	budget, err := parseFloatIfExists(r.FormValue("amount"))
	if err != nil {
		return nil, err
	}

	currId := r.FormValue("currency_id")

	var cur *models.Currency
	if currId != "" {
		cur = &models.Currency{
			ID: currId,
		}
	}

	return &models.Category{
		Name:  r.FormValue("name"),
		Color: r.FormValue("color"),
		// FIXME: add the kinds
		Kinds:          []models.TxKind{},
		BudgetAmount:   budget,
		BudgetCurrency: cur,
	}, nil
}

func dfltCategory(defaultCur *models.Currency) *models.Category {
	return &models.Category{
		BudgetCurrency: defaultCur,
		Kinds:          []models.TxKind{models.ExpenseTxKind},
	}
}

func getAllowedKindsForCategories() []models.TxKind {
	return slicesutils.Filter(models.GetTxKinds(), func(k models.TxKind) bool {
		return k != models.TransferTxKind && k != models.DebtTxKind
	})
}
