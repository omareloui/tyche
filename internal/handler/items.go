package handler

import (
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

func (h Handler) Items(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}

	cur, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched currencies")

	dfltCur := slicesutils.Find(cur, func(cur models.Currency) bool { return cur.Enum == models.DfltCurrency })
	dfltIt := dfltItem(dfltCur)
	fd := h.mapItemToFD(dfltIt, &errs.ValidationError{})

	its, err := h.repo.GetItems(r.Context())
	if err != nil {
		l.Error("fetching items", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched items")

	return views.Items(b, fd, its, cur), nil
}

func (h Handler) Item(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	it, err := h.repo.GetItem(r.Context(), id)
	if err != nil {
		l.Error("fetching item", zap.Error(err))
		return nil, err
	}
	l.Info("fetched item", zap.Any("item", it))

	return views.Item(it), nil
}

func (h Handler) CreateItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	// TODO: use a transaction here
	l := logger.FromCtx(r.Context())

	it, err := h.mapFDToItem(r)
	if err != nil {
		l.Error("parsing the form", zap.Error(err))
		return nil, err
	}
	it.ID = idgen.New()
	it.CreatedAt = time.Now()

	if err := h.sani.SanitizeStruct(it); err != nil {
		l.Error("sanitizing the form", zap.Error(err))
		return nil, errs.ErrSanitizer
	}

	l.Debug("create transaction", zap.Any("item", it))

	if valerr := h.valid.Validate(it); valerr != nil {
		l.Error("create item validation", zap.Error(valerr), zap.Any("errors", valerr.Errors))
		fd := h.mapItemToFD(it, valerr)

		cur, err := h.repo.GetCurrencies(r.Context())
		if err != nil {
			l.Error("fetching currencies", zap.Error(err))
			return nil, err
		}
		l.Debug("fetched currencies")

		return views.CreateItemForm(fd, cur), nil
	}

	if err := h.repo.CreateItem(r.Context(), it); err != nil {
		l.Error("creating item", zap.Error(err))
		return nil, err
	}
	l.Info("created item")

	cur, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched currencies")

	dfltCur := slicesutils.Find(cur, func(cur models.Currency) bool { return cur.Enum == models.DfltCurrency })
	fd := h.mapItemToFD(dfltItem(dfltCur), &errs.ValidationError{})
	it.Currency = *slicesutils.Find(cur, func(cur models.Currency) bool { return cur.ID == it.Currency.ID })

	if err = renderToBody(w, r, views.ItemOOB(it)); err != nil {
		l.Error("rendering item oob", zap.Error(err))
		return nil, err
	}
	l.Debug("rendered item oob")

	return views.CreateItemForm(fd, cur), nil
}

func (h Handler) GetEditItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	it, err := h.repo.GetItem(r.Context(), id)
	if err != nil {
		l.Error("fetching item", zap.Error(err))
		return nil, err
	}
	l.Info("fetched item", zap.Any("item", it))

	fd := h.mapItemToFD(it, &errs.ValidationError{})

	cur, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched currencies")

	return views.EditItem(fd, id, cur), nil
}

func (h Handler) EditItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	it, err := h.mapFDToItem(r)
	if err != nil {
		l.Error("parsing the form", zap.Error(err))
		return nil, err
	}
	it.ID = id

	l.Debug("update item", zap.Any("item", it))

	if err := h.repo.UpdateItem(r.Context(), it); err != nil {
		l.Error("update item", zap.Error(err))
		return nil, err
	}
	l.Info("updated item")

	return h.Item(w, r)
}

func (h Handler) DeleteItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())
	if err := h.repo.DeleteItem(r.Context(), id); err != nil {
		l.Error("deleting item", zap.Error(err))
		return nil, err
	}
	l.Info("deleted item")

	return nil, nil
}

func (h Handler) mapItemToFD(it *models.Item, err *errs.ValidationError) *views.ItemFD {
	return &views.ItemFD{
		Name: views.FDInput{Value: it.Name, Error: err.Errors.MsgFor("Name")},
		AmountFD: views.AmountFD{
			Amount:     views.FDInput{Value: formatIntIfNonZero(int(it.Price)), Error: err.Errors.MsgFor("Amount")},
			CurrencyID: views.FDInput{Value: it.Currency.ID, Error: err.Errors.MsgFor("Currency.ID")},
		},
	}
}

func (h Handler) mapFDToItem(r *http.Request) (*models.Item, error) {
	amount, err := parseFloatIfExists(r.FormValue("amount"))
	if err != nil {
		return nil, err
	}
	return &models.Item{
		Name:     r.FormValue("name"),
		Price:    amount,
		Currency: models.Currency{ID: r.FormValue("currency_id")},
	}, nil
}

func dfltItem(dfltCur *models.Currency) *models.Item {
	return &models.Item{
		Name:     "",
		Price:    0,
		Currency: models.Currency{ID: dfltCur.ID},
	}
}
