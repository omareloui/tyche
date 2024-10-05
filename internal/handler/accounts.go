package handler

import (
	"context"
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

	fd := h.mapAccToFD(defaultAccount(curs, ak), &errs.ValidationError{})
	return views.Accounts(balances, fd, ak, curs, accs), nil
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

	if err := a.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating account", zap.Any("account", a), zap.Any("errors", valerr.Errors))
			curs, ak, err := h.getCurrenciesAndAccKinds(r.Context())
			if err != nil {
				return nil, err
			}
			fd := h.mapAccToFD(a, &valerr)
			return views.CreateAccountForm(fd, ak, curs), nil
		}
		l.Error("sanitizing account", zap.Error(err))
		return nil, err
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
		fd := h.mapAccToFD(a, valerr)
		return views.CreateAccountForm(
			fd, ak, curs), nil
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

	if err := h.getAndRenderBalancesOOB(w, r); err != nil {
		return nil, err
	}

	fd := h.mapAccToFD(defaultAccount(curs, ak), &errs.ValidationError{})
	return views.CreateAccountForm(
		fd, ak, curs), nil
}

func (h *Handler) GetEditAccount(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
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

	curs, ak, err := h.getCurrenciesAndAccKinds(r.Context())
	if err != nil {
		return nil, err
	}

	fd := h.mapAccToFD(a, &errs.ValidationError{})
	return views.EditAccount(id, fd, ak, curs), nil
}

func (h *Handler) EditAccount(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	a, err := h.mapFDToAcc(r)
	if err != nil {
		l.Error("parsing formdata", zap.Error(err))
		return nil, err
	}
	l.Debug("parsed form data")

	a.ID = id

	if err := h.sani.SanitizeStruct(a); err != nil {
		l.Error("sanitizing account", zap.Error(err))
		return nil, err
	}
	l.Debug("sanitized account")

	if err := a.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating account", zap.Any("account", a), zap.Any("errors", valerr.Errors))
			curs, ak, err := h.getCurrenciesAndAccKinds(r.Context())
			if err != nil {
				return nil, err
			}
			a.Currency = *slicesutils.Find(curs, func(cur models.Currency) bool { return a.Currency.ID == cur.ID })
			a.Kind = *slicesutils.Find(ak, func(ak models.AccountKind) bool { return a.Kind.ID == ak.ID })
			fd := h.mapAccToFD(a, &valerr)
			return views.CreateAccountForm(fd, ak, curs), nil
		}
		l.Error("sanitizing account", zap.Error(err))
		return nil, err
	}
	l.Debug("validated account")

	err = h.repo.UpdateAccount(r.Context(), a)

	var valerr *errs.ValidationError
	if err != nil && errors.As(err, &valerr) {
		l.Error("duplication error", zap.Error(valerr), zap.Any("errors", valerr.Errors))
		curs, ak, err := h.getCurrenciesAndAccKinds(r.Context())
		if err != nil {
			return nil, err
		}

		a.Currency = *slicesutils.Find(curs, func(cur models.Currency) bool { return a.Currency.ID == cur.ID })
		a.Kind = *slicesutils.Find(ak, func(ak models.AccountKind) bool { return a.Kind.ID == ak.ID })

		fd := h.mapAccToFD(a, valerr)
		return views.EditAccount(id, fd, ak, curs), nil
	} else if err != nil {
		l.Error("updating account", zap.Error(err), zap.Any("account", a))
		return nil, err
	}
	l.Info("updated account", zap.Any("account", a))

	if err := h.getAndRenderBalancesOOB(w, r); err != nil {
		return nil, err
	}

	curs, ak, err := h.getCurrenciesAndAccKinds(r.Context())
	if err != nil {
		return nil, err
	}

	a.Currency = *slicesutils.Find(curs, func(cur models.Currency) bool { return cur.ID == a.Currency.ID })
	a.Kind = *slicesutils.Find(ak, func(k models.AccountKind) bool { return k.ID == a.Kind.ID })

	return views.Account(a), nil
}

func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	l := logger.FromCtx(r.Context())
	if err := h.repo.DeleteAccount(r.Context(), id); err != nil {
		l.Error("deleting account", zap.Error(err))
		return nil, err
	}
	l.Info("deleted account", zap.String("id", id))

	if err := h.getAndRenderBalancesOOB(w, r); err != nil {
		return nil, err
	}

	return nil, nil
}

func (h *Handler) SetDefaultAccount(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	l := logger.FromCtx(r.Context())
	if err := h.repo.SetDefaultAccountExpect(r.Context(), id); err != nil {
		l.Error("Setting as default", zap.Error(err))
		return nil, err
	}
	l.Info("Set as default", zap.String("id", id))
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
	return &models.Account{
		Name:               r.FormValue("name"),
		Kind:               models.AccountKind{ID: r.FormValue("kind_id")},
		IBAN:               r.FormValue("iban"),
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
