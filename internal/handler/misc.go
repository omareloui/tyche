package handler

import (
	"net/http"

	"github.com/a-h/templ"
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

	tx, err := h.mapFDToTransaction(r)
	if err != nil {
		return nil, err
	}

	currency := slicesutils.Find(curs, func(cur models.Currency) bool {
		return cur.ID == tx.Currency.ID
	})

	fd := &views.AmountFD{
		Amount:     views.FDInput{Value: r.FormValue("amount")},
		CurrencyID: views.FDInput{Value: currency.ID},
	}

	return views.AmountInputs(fd, h.repo.CurrenciesToMap(curs), currency.Enum.Symbol()), nil
}

func (h Handler) IbanInputs(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies")
		return nil, err
	}

	tx, err := h.mapFDToTransaction(r)
	if err != nil {
		return nil, err
	}

	currency := slicesutils.Find(curs, func(cur models.Currency) bool {
		return cur.ID == tx.Currency.ID
	})

	amount := r.FormValue("amount")
	if amount == "" {
		amount = "0"
	}

	fd := &views.IBANFD{
		IBAN:       views.FDInput{Value: r.FormValue("iban")},
		Amount:     views.FDInput{Value: amount},
		CurrencyID: views.FDInput{Value: currency.ID},
	}

	return views.IbanInputs(fd, "", h.repo.CurrenciesToMap(curs), currency), nil
}
