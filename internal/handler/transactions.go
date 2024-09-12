package handler

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-plans/internal/errs"
	"github.com/omareloui/financial-plans/internal/logger"
	"github.com/omareloui/financial-plans/internal/models"
	"github.com/omareloui/financial-plans/web/views"
	"go.uber.org/zap"
)

func (h Handler) Transactions(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	currencies, kinds, err := h.getCurrenciesAndKinds(r.Context())
	if err != nil {
		return nil, err
	}
	formdata, err := h.mapTransactionToFormData(defaultTransaction(currencies, kinds), nil)
	if err != nil {
		return nil, err
	}
	l := logger.FromCtx(r.Context())
	l.Debug("Create Transaction From Data", zap.Any("formdata", formdata))

	return views.Transactions(
		h.repo.CurrenciesToMap(currencies),
		h.repo.TransactionKindsToMap(kinds), formdata), nil
}

func (h Handler) CreateTransaction(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	transaction, err := h.mapFormDataToTransaction(r)
	if err != nil {
		return nil, err
	}

	transaction.CreatedAt = time.Now()

	l.Debug("Create transaction", zap.Any("transaction", transaction))

	currencies, kinds, err := h.getCurrenciesAndKinds(r.Context())
	if err != nil {
		return nil, err
	}

	formdata, err := h.mapTransactionToFormData(transaction, new(errs.ValidationError))
	if err != nil {
		return nil, err
	}

	if valerr := h.valid.Validate(transaction); valerr != nil {
		l.Error("Validation failed for creation", zap.Error(valerr), zap.Any("errors", valerr.Errors))
		formdata, err = h.mapTransactionToFormData(transaction, valerr)
		if err != nil {
			return nil, err
		}
	}

	l.Debug("Create transaction form data", zap.Any("formdata", formdata))

	// TODO:
	// - [ ] save the transaction
	// - [x] validate it and return if invalid
	// - [ ] add the created transaction to the list

	return views.CreateTransactionForm(
		h.repo.CurrenciesToMap(currencies),
		h.repo.TransactionKindsToMap(kinds), formdata), nil
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
		CurrencyID:  views.FormInputData{Value: t.CurrencyID, Error: err.Errors.MsgFor("CurrencyID")},
		KindID:      views.FormInputData{Value: t.KindID, Error: err.Errors.MsgFor("KindID")},
		IsRecurrent: views.FormInputData{Value: formatBooleanIfNonZero(t.IsRecurrent), Error: err.Errors.MsgFor("IsRecurrent")},
	}, nil
}

func (h Handler) mapFormDataToTransaction(r *http.Request) (*models.Transaction, error) {
	amount, err := parseIntIfExists(r.FormValue("amount"))
	if err != nil {
		return nil, err
	}

	return &models.Transaction{
		Amount:      int64(amount),
		CurrencyID:  r.FormValue("currency_id"),
		KindID:      r.FormValue("kind_id"),
		IsRecurrent: parseBoolean(r.FormValue("is_recurring")),
	}, nil
}

func defaultTransaction(currencies []models.Currency, kinds []models.TransactionKind) *models.Transaction {
	defaultCurrency := "EGP"
	defaultKind := "EXPENSES"
	defaultCurrencyIdx := slices.IndexFunc(currencies, func(cur models.Currency) bool {
		fmt.Println(cur)
		return cur.Currency == defaultCurrency
	})
	defaultKindIdx := slices.IndexFunc(kinds, func(knd models.TransactionKind) bool {
		return knd.Kind == defaultKind
	})

	return &models.Transaction{
		Amount:      0,
		CurrencyID:  currencies[defaultCurrencyIdx].ID,
		KindID:      kinds[defaultKindIdx].ID,
		IsRecurrent: true,
	}
}
