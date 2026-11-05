package handler

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
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
	return views.Categories(claims, b, h.mapCategoryToFD(dfltCategory(&claims.DefaultCurrency, knds), knds, &errs.ValidationError{}), knds, cats, currs), nil
}

func (h Handler) CategoriesBudget(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	claims := getClaims(r.Context())
	month, hidden, err := summaryParams(r)
	if err != nil {
		return nil, err
	}

	bs, err := h.repo.GetBudgetSummary(r.Context(), month, &claims.DefaultCurrency, hidden)
	if err != nil {
		return nil, err
	}
	return views.CategoriesBudget(bs), nil
}

// summaryParams are a summary's month (zero for the current one) and hidden
// ids, from the query string.
func summaryParams(r *http.Request) (month time.Time, hidden []string, err error) {
	if v := r.URL.Query().Get("month"); v != "" {
		if month, err = time.Parse("2006-01", v); err != nil {
			return month, nil, resperr.New(http.StatusUnprocessableEntity, "invalid month")
		}
	}
	for id := range strings.SplitSeq(r.URL.Query().Get("hide"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			hidden = append(hidden, id)
		}
	}
	return month, hidden, nil
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

	currs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}

	claims := getClaims(r.Context())

	knds := getAllowedKindsForCategories()
	if err := cat.Validate(h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("create category validation", zap.Error(valerr), zap.Any("errors", valerr.Errors))
			return views.CreateCategoryForm(h.mapCategoryToFD(cat, knds, &valerr), knds, currs, &claims.DefaultCurrency), nil
		}
		l.Error("sanitizing category", zap.Error(err))
		return nil, err
	}

	if err := h.repo.CreateCategory(r.Context(), cat); err != nil {
		l.Error("creating category", zap.Error(err))
		return nil, err
	}
	l.Info("created category")
	addHXTrigger(w, views.BudgetChangedEvent)
	drawerCreated(w)

	if err = renderToBody(w, r, views.CategoryOOB(cat)); err != nil {
		l.Error("rendering category oob", zap.Error(err))
		return nil, err
	}
	l.Debug("rendered category oob")

	return views.CreateCategoryForm(h.mapCategoryToFD(dfltCategory(&claims.DefaultCurrency, knds), knds, &errs.ValidationError{}), knds, currs, &claims.DefaultCurrency), nil
}

// QuickCreateCategory creates a category from a combobox's search, allowing
// the "kind" query's tx kind, and responds with its combobox option. An
// existing category with the same name is reused, gaining the kind if needed.
func (h Handler) QuickCreateCategory(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	kind := models.GetTxKindFromString(r.URL.Query().Get("kind"))
	if !slices.Contains(getAllowedKindsForCategories(), kind) {
		return nil, resperr.New(http.StatusBadRequest, "invalid kind")
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		return nil, resperr.New(http.StatusBadRequest, "name is required")
	}

	cats, err := h.repo.GetCategories(r.Context())
	if err != nil {
		l.Error("fetching categories", zap.Error(err))
		return nil, err
	}

	if cat := slicesutils.Find(cats, func(c models.Category) bool { return strings.EqualFold(c.Name, name) }); cat != nil {
		if !cat.HasKind(kind) {
			cat.Kinds = append(cat.Kinds, kind)
			if err := h.repo.UpdateCategory(r.Context(), cat); err != nil {
				l.Error("adding kind to category", zap.Error(err))
				return nil, err
			}
			l.Info("added kind to existing category", zap.String("id", cat.ID))
		}
		return views.CategoryComboOption(cat), nil
	}

	cat := &models.Category{
		ID:        idgen.New(),
		Name:      name,
		Color:     models.GetRandomCategoryColor(),
		Kinds:     []models.TxKind{kind},
		CreatedAt: time.Now(),
	}
	if err := cat.Validate(h.valid); err != nil {
		l.Error("quick create category validation", zap.Error(err))
		return nil, resperr.New(http.StatusBadRequest, err.Error())
	}

	if err := h.repo.CreateCategory(r.Context(), cat); err != nil {
		l.Error("creating category", zap.Error(err))
		return nil, err
	}
	l.Info("quick created category")

	return views.CategoryComboOption(cat), nil
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

	fd := h.mapCategoryToFD(cat, knds, &errs.ValidationError{})
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

	currs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}

	claims := getClaims(r.Context())

	if err := cat.Validate(h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			knds := getAllowedKindsForCategories()
			l.Error("update category validation", zap.Error(valerr), zap.Any("errors", valerr.Errors))
			return views.EditCategoryForm(h.mapCategoryToFD(cat, knds, &valerr), id, knds, currs, &claims.DefaultCurrency), nil
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
	addHXTrigger(w, views.BudgetChangedEvent)
	drawerSaved(w, "#"+views.CategoryID(id), "outerHTML")

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
	w.Header().Set("HX-Trigger", views.BudgetChangedEvent)

	return nil, nil
}

func (h Handler) mapCategoryToFD(cat *models.Category, kinds []models.TxKind, err *errs.ValidationError) *views.CategoryFD {
	var budgetCurrID string
	if cat.BudgetCurrency != nil {
		budgetCurrID = cat.BudgetCurrency.ID
	}

	knds := make(form.TxKindFDInputs, len(cat.Kinds))

	for _, kind := range kinds {
		for _, catKind := range cat.Kinds {
			if catKind == kind {
				knds[kind] = form.FDInput{Value: catKind.String(), Error: err.Errors.MsgFor("Kinds")}
			}
		}
		if _, ok := knds[kind]; !ok {
			knds[kind] = form.FDInput{Error: err.Errors.MsgFor("Kinds")}
		}
	}

	return &views.CategoryFD{
		Name:  form.FDInput{Value: cat.Name, Error: err.Errors.MsgFor("Name")},
		Color: form.FDInput{Value: cat.Color, Error: err.Errors.MsgFor("Color")},
		Kinds: knds,
		BudgetAmountFD: &form.AmountFD{
			Amount:     form.FDInput{Value: formatFloatIfNonZero(cat.BudgetAmount), Error: err.Errors.MsgFor("BudgetAmount")},
			CurrencyID: form.FDInput{Value: budgetCurrID, Error: err.Errors.MsgFor("BudgetCurrency.ID")},
		},
	}
}

func (h Handler) mapFDToCategory(r *http.Request) (*models.Category, error) {
	l := logger.FromCtx(r.Context())
	l.Debug("parsing the category body")

	budget, err := parseFloatIfExists(r.FormValue("amount"))
	if err != nil {
		return nil, err
	}

	currID := r.FormValue("currency_id")

	// The currency is always picked; without an amount there's no budget.
	var cur *models.Currency
	if currID != "" && budget != 0 {
		cur = &models.Currency{
			ID: currID,
		}
	}

	formKinds := r.Form["kind"]
	kinds := []models.TxKind{}

	for _, kind := range formKinds {
		txKind := models.GetTxKindFromString(kind)
		if txKind != models.NilTxKind {
			kinds = append(kinds, txKind)
		}
	}

	return &models.Category{
		Name:           r.FormValue("name"),
		Color:          r.FormValue("color"),
		Kinds:          kinds,
		BudgetAmount:   budget,
		BudgetCurrency: cur,
	}, nil
}

func dfltCategory(defaultCur *models.Currency, knds []models.TxKind) *models.Category {
	kinds := make([]models.TxKind, len(knds))
	return &models.Category{
		BudgetCurrency: defaultCur,
		Kinds:          kinds,
	}
}

func getAllowedKindsForCategories() []models.TxKind {
	return slicesutils.Filter(models.GetTxKinds(), func(k models.TxKind) bool {
		return k != models.NilTxKind && k != models.TransferTxKind && k != models.DebtTxKind && k != models.RefundTxKind &&
			k != models.AdjustmentTxKind
	})
}
