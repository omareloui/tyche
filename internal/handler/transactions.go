package handler

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-plans/internal/config"
	"github.com/omareloui/financial-plans/internal/errs"
	"github.com/omareloui/financial-plans/internal/idgen"
	"github.com/omareloui/financial-plans/internal/logger"
	"github.com/omareloui/financial-plans/internal/models"
	"github.com/omareloui/financial-plans/internal/slicesutils"
	"github.com/omareloui/financial-plans/web/views"
	"go.uber.org/zap"
)

func (h Handler) Transactions(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	currencies, kinds, err := h.getCurrenciesAndKinds(r.Context())
	if err != nil {
		return nil, err
	}
	formdata, err := h.mapTransactionToFormData(defaultTransaction(currencies, kinds), new(errs.ValidationError))
	if err != nil {
		return nil, err
	}

	l := logger.FromCtx(r.Context())

	transactions, err := h.repo.GetTransactions(r.Context())
	if err != nil {
		l.Error("fetch transaction from db", zap.Error(err))
		return nil, err
	}

	l.Debug("transaction from data", zap.Any("formdata", formdata))
	l.Debug("transactions count", zap.Int("transactions_count", len(transactions)))

	defaultCurrency := slicesutils.Find(currencies, func(cur models.Currency) bool { return cur.Currency == config.GetDefaultCurrency() })
	comp := views.Transactions(formdata, transactions,
		h.repo.CurrenciesToMap(currencies),
		h.repo.TransactionKindsToMap(kinds),
		defaultCurrency.Symbol)

	return comp, nil
}

func (h Handler) CreateTransaction(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	tx, err := h.mapFormDataToTransaction(r)
	if err != nil {
		return nil, err
	}
	tx.ID = idgen.New()
	tx.CreatedAt = time.Now()

	if err := h.sani.SanitizeStruct(tx); err != nil {
		return nil, errs.ErrSanitizer
	}

	l.Debug("create transaction", zap.Any("transaction", tx))

	currencies, kinds, err := h.getCurrenciesAndKinds(r.Context())
	if err != nil {
		l.Error("fetching currencies and kinds")
		return nil, err
	}
	defaultCurrency := slicesutils.Find(currencies, func(cur models.Currency) bool { return cur.Currency == config.GetDefaultCurrency() })

	if valerr := h.valid.Validate(tx); valerr != nil {
		l.Error("create transaction validation", zap.Error(valerr), zap.Any("errors", valerr.Errors))
		formdata, err := h.mapTransactionToFormData(tx, valerr)
		if err != nil {
			return nil, err
		}

		comp := views.CreateTransactionForm(formdata,
			h.repo.CurrenciesToMap(currencies),
			h.repo.TransactionKindsToMap(kinds),
			defaultCurrency.Symbol)

		return comp, nil
	}

	if err := h.repo.CreateTransaction(r.Context(), tx); err != nil {
		l.Error("creating transaction", zap.Error(err))
		return nil, err
	}
	l.Info("created transaction")

	c := slicesutils.Find(currencies, func(cur models.Currency) bool { return cur.ID == tx.Currency.ID })
	tx.Currency = *c
	k := slicesutils.Find(kinds, func(k models.TransactionKind) bool { return k.ID == tx.Kind.ID })
	tx.Kind = *k

	if err = renderToBody(w, r, views.TransactionOOB(tx)); err != nil {
		l.Error("rendering OOB transaction")
		return nil, err
	}

	formdata, err := h.mapTransactionToFormData(defaultTransaction(currencies, kinds), new(errs.ValidationError))
	if err != nil {
		return nil, err
	}

	comp := views.CreateTransactionForm(formdata,
		h.repo.CurrenciesToMap(currencies),
		h.repo.TransactionKindsToMap(kinds),
		defaultCurrency.Symbol)

	return comp, nil
}

func (h Handler) AmountInputs(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies")
		return nil, err
	}

	tx, err := h.mapFormDataToTransaction(r)
	if err != nil {
		return nil, err
	}

	currency := slicesutils.Find(curs, func(cur models.Currency) bool {
		return cur.ID == tx.Currency.ID
	})

	formdata := &views.TransactionFormData{
		Amount:     views.FormInputData{Value: r.FormValue("amount")},
		CurrencyID: views.FormInputData{Value: currency.ID},
	}

	return views.AmountInputs(formdata, h.repo.CurrenciesToMap(curs), currency.Symbol), nil
}

func (h Handler) Transaction(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		return nil, err
	}

	return views.Transaction(tx), nil
}

func (h Handler) GetEditTransaction(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		return nil, err
	}

	if err := h.sani.SanitizeStruct(tx); err != nil {
		return nil, errs.ErrSanitizer
	}

	l.Debug("get edit transaction", zap.Any("transaction", tx))

	currencies, kinds, err := h.getCurrenciesAndKinds(r.Context())
	if err != nil {
		l.Error("fetching currencies and kinds")
		return nil, err
	}

	fd, err := h.mapTransactionToFormData(tx, new(errs.ValidationError))
	if err != nil {
		return nil, err
	}

	return views.EditTransaction(tx, fd,
		h.repo.CurrenciesToMap(currencies),
		h.repo.TransactionKindsToMap(kinds)), nil
}

func (h Handler) EditTransaction(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	tx, err := h.mapFormDataToTransaction(r)
	if err != nil {
		return nil, err
	}
	tx.ID = id

	if err := h.sani.SanitizeStruct(tx); err != nil {
		return nil, errs.ErrSanitizer
	}

	l.Debug("update transaction", zap.Any("new transaction", tx))

	if valerr := h.valid.Validate(tx); valerr != nil {
		l.Error("update transaction validation", zap.Error(valerr), zap.Any("errors", valerr.Errors))
		fd, err := h.mapTransactionToFormData(tx, valerr)
		if err != nil {
			return nil, err
		}
		fd.Amount.Value = r.FormValue("amount")

		currencies, kinds, err := h.getCurrenciesAndKinds(r.Context())
		if err != nil {
			l.Error("fetching currencies and kinds")
			return nil, err
		}

		return views.EditTransaction(tx, fd,
			h.repo.CurrenciesToMap(currencies),
			h.repo.TransactionKindsToMap(kinds)), nil
	}

	if err := h.repo.UpdateTransaction(r.Context(), tx); err != nil {
		l.Error("updating transaction", zap.Error(err))
		return nil, err
	}
	l.Info("updated transaction")

	tx, err = h.repo.GetTransaction(r.Context(), tx.ID)
	if err != nil {
		return nil, err
	}

	return views.Transaction(tx), nil
}

func (h Handler) getCurrenciesAndKinds(ctx context.Context) ([]models.Currency, []models.TransactionKind, error) {
	curs, err := h.repo.GetCurrencies(ctx)
	if err != nil {
		return nil, nil, err
	}
	knds, err := h.repo.GetTransactionKinds(ctx)
	if err != nil {
		return nil, nil, err
	}
	return curs, knds, nil
}

func (h Handler) mapTransactionToFormData(t *models.Transaction, err *errs.ValidationError) (*views.TransactionFormData, error) {
	return &views.TransactionFormData{
		Amount:      views.FormInputData{Value: formatIntIfNonZero(int(t.Amount)), Error: err.Errors.MsgFor("Amount")},
		CurrencyID:  views.FormInputData{Value: t.Currency.ID, Error: err.Errors.MsgFor("Currency.ID")},
		KindID:      views.FormInputData{Value: t.Kind.ID, Error: err.Errors.MsgFor("Kind.ID")},
		IsRecurrent: views.FormInputData{Value: formatBooleanIfNonZero(t.IsRecurrent), Error: err.Errors.MsgFor("IsRecurrent")},
		ScheduledTo: views.FormInputData{Value: formatDateOnlyIfNonZero(t.ScheduledTo), Error: err.Errors.MsgFor("ScheduledTo")},
	}, nil
}

func (h Handler) mapFormDataToTransaction(r *http.Request) (*models.Transaction, error) {
	amount, err := parseIntIfExists(r.FormValue("amount"))
	if err != nil {
		return nil, err
	}

	ScheduledTo, err := parseDateOnlyIfExists(r.FormValue("scheduled_to"))
	if err != nil {
		return nil, err
	}

	return &models.Transaction{
		Amount:      int64(amount),
		Currency:    models.Currency{ID: r.FormValue("currency_id")},
		Kind:        models.TransactionKind{ID: r.FormValue("kind_id")},
		IsRecurrent: parseBoolean(r.FormValue("is_recurrent")),
		ScheduledTo: ScheduledTo,
	}, nil
}

func defaultTransaction(currencies []models.Currency, kinds []models.TransactionKind) *models.Transaction {
	defaultCurrency := config.GetDefaultCurrency()
	defaultKind := "EXPENSES"
	defaultCurrencyIdx := slices.IndexFunc(currencies, func(cur models.Currency) bool {
		return cur.Currency == defaultCurrency
	})
	defaultKindIdx := slices.IndexFunc(kinds, func(knd models.TransactionKind) bool {
		return knd.Kind == defaultKind
	})

	return &models.Transaction{
		Amount:      0,
		Currency:    models.Currency{ID: currencies[defaultCurrencyIdx].ID},
		Kind:        models.TransactionKind{ID: kinds[defaultKindIdx].ID},
		IsRecurrent: false,
		ScheduledTo: time.Now(),
	}
}
