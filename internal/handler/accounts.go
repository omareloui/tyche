package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
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

func (h *Handler) Accounts(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	balances, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}

	curs, ak, err := h.getCurrenciesAndAccKinds(r.Context())
	if err != nil {
		return nil, err
	}

	accs, err := h.repo.GetAccounts(r.Context())
	if err != nil {
		l.Error("fetching accounts", zap.Error(err))
		return nil, err
	}
	l.Info("fetched accounts", zap.Int("accounts_length", len(accs)))

	defaultCurrency := slicesutils.Find(curs, func(cur models.Currency) bool { return cur.Enum == models.DfltCurrency })

	fd := h.mapAccToFD(defaultAccount(curs, ak), &errs.ValidationError{})
	return views.Accounts(
		balances, fd, h.repo.AccountKindsToMap(ak), h.repo.CurrenciesToMap(curs),
		defaultCurrency, accs), nil
}

func (h *Handler) Account(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	l := logger.FromCtx(r.Context())

	a, err := h.repo.GetAccount(r.Context(), id)
	if err != nil {
		l.Error("fetching account", zap.Error(err))
		return nil, err
	}
	l.Info("fetched account")

	return views.Account(a), nil
}

func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	a, err := h.mapFDToAcc(r)
	if err != nil {
		l.Error("parsing formdata", zap.Error(err))
		return nil, err
	}
	l.Debug("parsed form data")

	a.ID = idgen.New()
	a.CreatedAt = time.Now()

	if err := h.sani.SanitizeStruct(a); err != nil {
		l.Error("sanitizing account", zap.Error(err))
		return nil, err
	}
	l.Debug("sanitized account")

	if err := h.sani.SanitizeStruct(a); err != nil {
		l.Error("sanitizing account", zap.Error(err))
		return nil, err
	}
	l.Debug("sanitized account")

	if valerr := h.valid.Validate(a); valerr != nil {
		l.Error("validating account", zap.Any("account", a), zap.Any("errors", valerr.Errors))
		curs, ak, err := h.getCurrenciesAndAccKinds(r.Context())
		if err != nil {
			return nil, err
		}

		defaultCurrency := slicesutils.Find(curs, func(cur models.Currency) bool { return cur.Enum == models.DfltCurrency })

		fd := h.mapAccToFD(a, valerr)
		return views.CreateAccountForm(
			fd, h.repo.AccountKindsToMap(ak), h.repo.CurrenciesToMap(curs),
			defaultCurrency), nil
	}
	l.Debug("validated account")

	err = h.repo.CreateAccount(r.Context(), a)

	var valerr *errs.ValidationError
	if err != nil && errors.As(err, &valerr) {
		l.Error("duplication error", zap.Error(valerr), zap.Any("errors", valerr.Errors))
		curs, ak, err := h.getCurrenciesAndAccKinds(r.Context())
		if err != nil {
			return nil, err
		}
		defaultCurrency := slicesutils.Find(curs, func(cur models.Currency) bool { return cur.Enum == models.DfltCurrency })
		fd := h.mapAccToFD(a, valerr)
		return views.CreateAccountForm(
			fd, h.repo.AccountKindsToMap(ak), h.repo.CurrenciesToMap(curs),
			defaultCurrency), nil
	} else if err != nil {
		l.Error("creating account", zap.Error(err), zap.Any("account", a))
		return nil, err
	}
	l.Info("created account", zap.Any("account", a))

	curs, ak, err := h.getCurrenciesAndAccKinds(r.Context())
	if err != nil {
		return nil, err
	}

	a.Currency = *slicesutils.Find(curs, func(cur models.Currency) bool { return cur.ID == a.Currency.ID })
	a.Kind = *slicesutils.Find(ak, func(k models.AccountKind) bool { return k.ID == a.Kind.ID })

	if err := renderToBody(w, r, views.AccountOOB(a)); err != nil {
		l.Error("rendering account oob", zap.Error(err))
		return nil, err
	}
	l.Debug("rendered account oob")

	defaultCurrency := slicesutils.Find(curs, func(cur models.Currency) bool { return cur.Enum == models.DfltCurrency })

	fd := h.mapAccToFD(defaultAccount(curs, ak), &errs.ValidationError{})
	return views.CreateAccountForm(
		fd, h.repo.AccountKindsToMap(ak), h.repo.CurrenciesToMap(curs),
		defaultCurrency), nil
}

func (h *Handler) GetEditAccount(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return nil, nil
}

func (h *Handler) EditAccount(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return nil, nil
}

func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return nil, nil
}

func (h Handler) getCurrenciesAndAccKinds(ctx context.Context) ([]models.Currency, []models.AccountKind, error) {
	l := logger.FromCtx(ctx)

	curs, err := h.repo.GetCurrencies(ctx)
	if err != nil {
		l.Error("error fetching currencies", zap.Error(err))
		return nil, nil, err
	}
	knds, err := h.repo.GetAccountKinds(ctx)
	if err != nil {
		l.Error("error fetching account kinds", zap.Error(err))
		return nil, nil, err
	}
	l.Debug("fetched currencies and account kinds")
	return curs, knds, nil
}

func (h *Handler) mapFDToAcc(r *http.Request) (*models.Account, error) {
	amount, err := parseFloatIfExists(r.FormValue("amount"))
	if err != nil {
		return nil, err
	}

	return &models.Account{
		Name:               r.FormValue("name"),
		Kind:               models.AccountKind{ID: r.FormValue("kind_id")},
		IBAN:               r.FormValue("iban"),
		Amount:             amount,
		CardNumber:         r.FormValue("card_number"),
		Note:               r.FormValue("note"),
		Currency:           models.Currency{ID: r.FormValue("currency_id")},
		IsArchive:          parseBoolean(r.FormValue("is_archive")),
		ExcludeFromBalance: parseBoolean(r.FormValue("exclude_from_balance")),
	}, nil
}

func (h *Handler) mapAccToFD(acc *models.Account, errs *errs.ValidationError) *views.AccountFD {
	return &views.AccountFD{
		IBANFD: views.IBANFD{
			Amount:     views.FDInput{Value: strconv.FormatFloat(acc.Amount, 'f', -1, 64), Error: errs.Errors.MsgFor("Amount")},
			CurrencyID: views.FDInput{Value: acc.Currency.ID, Error: errs.Errors.MsgFor("Currency.ID")},
			IBAN:       views.FDInput{Value: acc.IBAN, Error: errs.Errors.MsgFor("IBAN")},
		},
		Name:               views.FDInput{Value: acc.Name, Error: errs.Errors.MsgFor("Name")},
		KindID:             views.FDInput{Value: acc.Kind.ID, Error: errs.Errors.MsgFor("Kind.ID")},
		CardNumber:         views.FDInput{Value: acc.CardNumber, Error: errs.Errors.MsgFor("CardNumber")},
		Note:               views.FDInput{Value: acc.Note, Error: errs.Errors.MsgFor("Note")},
		IsArchive:          views.FDInput{Value: formatBooleanIfNonZero(acc.IsArchive), Error: errs.Errors.MsgFor("IsArchive")},
		ExcludeFromBalance: views.FDInput{Value: formatBooleanIfNonZero(acc.ExcludeFromBalance), Error: errs.Errors.MsgFor("IsArchive")},
	}
}

func defaultAccount(curs []models.Currency, ak []models.AccountKind) *models.Account {
	return &models.Account{
		Currency:           *slicesutils.Find(curs, func(cur models.Currency) bool { return cur.Enum == models.DfltCurrency }),
		Kind:               *slicesutils.Find(ak, func(k models.AccountKind) bool { return k.Enum == models.DfltAccountKind }),
		IsArchive:          false,
		ExcludeFromBalance: false,
	}
}
