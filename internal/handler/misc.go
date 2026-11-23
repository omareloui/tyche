package handler

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
)

func (h Handler) AmountInputsWithCurrency(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
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

	claims := getClaims(r.Context())

	fd := &form.AmountFD{
		Amount:     form.FDInput{Value: r.FormValue("amount")},
		CurrencyID: form.FDInput{Value: currency.ID},
	}

	if fd.CurrencyID.Value == "" {
		fd.CurrencyID.Value = claims.DefaultCurrency.ID
	}

	return form.AmountInputsWithCurrency(fd, r.PathValue("id"), curs, &claims.DefaultCurrency), nil
}

func (h Handler) AmountInputsWithAccount(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
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

	fd := &form.AccountIFD{
		Amount:    form.FDInput{Value: r.FormValue("amount")},
		AccountID: form.FDInput{Value: acc.ID},
	}

	claims := getClaims(r.Context())
	return form.AmountInputsWithAccount(fd, r.FormValue("id"), accs, &claims.DefaultCurrency), nil
}

func (h Handler) TxInputs(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	accs, knds, cats, cps, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}
	fd, err := h.parseTransactionAmountsAndKindFD(r, accs, knds)
	if err != nil {
		return nil, err
	}
	claims := getClaims(r.Context())
	return views.TransactionAmountsAndKindFormBody(fd, r.PathValue("id"), knds, cats, accs, cps, &claims.DefaultCurrency), nil
}

func (h Handler) TxInputsSwitch(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	accs, knds, cats, cps, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}
	if err := r.ParseForm(); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "invalid form")
	}
	// Swap the accounts (and their units) before parsing, then work the
	// received amount out again as if the from account was picked.
	if toAccId := r.Form.Get("to_account_id"); toAccId != "" {
		accId := r.Form.Get("account_id")
		r.Form.Set("account_id", toAccId)
		r.Form.Set("to_account_id", accId)
		unit, toUnit := r.Form.Get("amount_unit"), r.Form.Get("to_amount_unit")
		r.Form.Set("amount_unit", toUnit)
		r.Form.Set("to_amount_unit", unit)
		r.Header.Set("Hx-Trigger", "account_id-"+r.PathValue("id"))
	}
	fd, err := h.parseTransactionAmountsAndKindFD(r, accs, knds)
	if err != nil {
		return nil, err
	}

	claims := getClaims(r.Context())
	return views.TransactionAmountsAndKindFormBody(fd, r.PathValue("id"), knds, cats, accs, cps, &claims.DefaultCurrency), nil
}
