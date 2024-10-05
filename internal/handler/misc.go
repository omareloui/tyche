package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
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
	accs, err := h.repo.GetAccounts(r.Context())
	if err != nil {
		return nil, err
	}

	knds, err := h.repo.GetTransactionKinds(r.Context())
	if err != nil {
		return nil, err
	}

	kndId := r.FormValue("kind_id")
	knd := slicesutils.Find(knds, func(k models.TransactionKind) bool { return k.ID == kndId })
	if knd == nil {
		return nil, errs.ErrInvalidID
	}

	cats, err := h.repo.GetCategories(r.Context())
	if err != nil {
		return nil, err
	}

	accId := r.FormValue("account_id")

	fd := &views.TransactionAmountsAndKindFD{
		AccountIFD: views.AccountIFD{
			Amount:    views.FDInput{Value: r.FormValue("amount")},
			AccountID: views.FDInput{Value: accId},
		},
		KindID: views.FDInput{Value: kndId},
	}

	trigger := r.Header.Get("Hx-Trigger")

	if knd.Kind == models.TransferTxKind {
		toAccId := r.FormValue("to_account_id")
		toAmt := r.FormValue("to_amount")

		acc := slicesutils.Find(accs, func(a models.Account) bool { return a.ID == accId })
		toAcc := slicesutils.Find(accs, func(a models.Account) bool { return a.ID == toAccId })

		// Set to account
		if toAccId == "" || (strings.HasPrefix(trigger, "account_id") && toAccId == accId) {
			if nAcc := slicesutils.Find(accs, func(a models.Account) bool { return a.ID != accId }); nAcc != nil {
				toAccId = nAcc.ID
				toAcc = nAcc
			}
		}

		// Set from account
		if strings.HasPrefix(trigger, "to_account_id") && toAccId == accId {
			if nAcc := slicesutils.Find(accs, func(a models.Account) bool { return a.ID != toAccId }); nAcc != nil {
				fd.AccountIFD.AccountID.Value = nAcc.ID
				acc = nAcc
			}
		}

		// Set to amount
		if strings.HasPrefix(trigger, "amount") ||
			strings.HasPrefix(trigger, "kind_id") ||
			strings.HasPrefix(trigger, "to_account_id") ||
			strings.HasPrefix(trigger, "account_id") {
			fxRate := 1.0
			if acc != nil && toAcc != nil {
				fxRate = models.GetFxRate(acc.Currency, toAcc.Currency)
			}
			amt, _ := parseFloatIfExists(fd.AccountIFD.Amount.Value)
			toAmt = strconv.FormatFloat(amt/fxRate, 'f', 2, 64)
		}

		fd.ToAccountIFD = &views.AccountIFD{
			Amount:    views.FDInput{Value: toAmt},
			AccountID: views.FDInput{Value: toAccId},
		}

	} else {
		fd.CategoryID = &views.FDInput{Value: r.FormValue("category_id")}
	}

	return views.TransactionAmountsAndKindFormBody(fd, r.PathValue("id"), knds, cats, accs), nil
}
