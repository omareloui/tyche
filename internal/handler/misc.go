package handler

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-planner/internal/errs"
	"github.com/omareloui/financial-planner/internal/logger"
	"github.com/omareloui/financial-planner/internal/models"
	"github.com/omareloui/financial-planner/internal/slicesutils"
	"github.com/omareloui/financial-planner/web/views"
)

func (h Handler) AmountInputs(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies")
		return nil, err
	}

	curId := r.FormValue("currency_id")
	if curId == "" {
		return nil, errs.ErrInvalidID
	}

	currency := slicesutils.Find(curs, func(cur models.Currency) bool {
		return cur.ID == curId
	})

	if currency == nil {
		return nil, errs.ErrInvalidID
	}

	fd := &views.AmountFD{
		Amount:     views.FDInput{Value: r.FormValue("amount")},
		CurrencyID: views.FDInput{Value: currency.ID},
	}

	return views.AmountInputs(fd, "", curs), nil
}

func (h Handler) AccountInputs(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	accs, err := h.repo.GetAccounts(r.Context())
	if err != nil {
		l.Error("fetching currencies")
		return nil, err
	}

	accId := r.FormValue("account_id")
	if accId == "" {
		return nil, errs.ErrInvalidID
	}

	acc := slicesutils.Find(accs, func(a models.Account) bool { return a.ID == accId })

	if acc == nil {
		return nil, errs.ErrInvalidID
	}

	fd := &views.AccountIFD{
		Amount:    views.FDInput{Value: r.FormValue("amount")},
		AccountID: views.FDInput{Value: acc.ID},
	}

	return views.AccountInputs(fd, "", accs), nil
}

func (h Handler) IbanInputs(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies")
		return nil, err
	}

	curId := r.FormValue("currency_id")
	if curId == "" {
		return nil, errs.ErrInvalidID
	}

	currency := slicesutils.Find(curs, func(cur models.Currency) bool {
		return cur.ID == curId
	})

	if currency == nil {
		return nil, errs.ErrInvalidID
	}

	fd := &views.IBANFD{
		IBAN:       views.FDInput{Value: r.FormValue("iban")},
		CurrencyID: views.FDInput{Value: currency.ID},
	}

	return views.IbanInputs(fd, "", curs), nil
}

func (h Handler) CategoriesInputs(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	kndId := r.FormValue("kind_id")
	if kndId == "" {
		return nil, errs.ErrInvalidID
	}

	catId := r.FormValue("category_id")

	knds, err := h.repo.GetTransactionKinds(r.Context())
	if err != nil {
		return nil, err
	}

	cats, err := h.repo.GetCategories(r.Context())
	if err != nil {
		return nil, err
	}

	fmt.Printf("kndId: %s, catdId: %s\n", kndId, catId)

	kndsCats := []models.Category{}

	for _, cat := range cats {
		if cat.Kind.ID == kndId {
			kndsCats = append(kndsCats, cat)
		}
	}

	fd := &views.CategoryAndKindsFD{
		KindID:     views.FDInput{Value: kndId},
		CategoryID: views.FDInput{Value: catId},
	}

	return views.KindCategoryInputs(fd, "", knds, kndsCats), nil
}
