package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

func (h *Handler) Accounts(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	balances, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}

	_, shownCurs, knds, cps, err := h.getAccDeps(r.Context())
	if err != nil {
		return nil, err
	}

	accs, err := h.repo.GetAccounts(r.Context())
	if err != nil {
		l.Error("fetching accounts", zap.Error(err))
		return nil, err
	}
	l.Info("fetched accounts", zap.Int("accounts_length", len(accs)))

	claims := getClaims(r.Context())
	fd := h.mapAccToFD(defaultAccount(claims, knds), &errs.ValidationError{}, knds)
	return views.Accounts(claims, balances, fd, knds, shownCurs, accs, cps), nil
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

	curs, shownCurs, knds, cps, err := h.getAccDeps(r.Context())
	if err != nil {
		return nil, err
	}

	a.Currency = *slicesutils.Find(curs, func(cur models.Currency) bool { return cur.ID == a.Currency.ID })
	a.Kind = *slicesutils.Find(knds, func(k models.AccountKind) bool { return k.ID == a.Kind.ID })

	if a.Counterparty != nil {
		if cp := slicesutils.Find(cps, func(cp models.Counterparty) bool { return cp.ID == a.Counterparty.ID }); cp != nil {
			a.Counterparty = cp
		}
	}

	if a.Kind.Enum == models.AccKindDebt && a.Counterparty != nil {
		a.Name = a.Counterparty.Name
	}

	claims := getClaims(r.Context())

	if err := a.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating account", zap.Any("account", a), zap.Any("errors", valerr.Errors))
			return views.CreateAccountForm(h.mapAccToFD(a, &valerr, knds), knds, shownCurs, cps, &claims.DefaultCurrency), nil
		}
		l.Error("sanitizing account", zap.Error(err))
		return nil, err
	}
	l.Debug("validated account")

	err = h.repo.CreateAccount(r.Context(), a)

	var valerr *errs.ValidationError
	if err != nil && errors.As(err, &valerr) {
		l.Error("duplication error", zap.Error(valerr), zap.Any("errors", valerr.Errors))
		return views.CreateAccountForm(h.mapAccToFD(a, valerr, knds), knds, shownCurs, cps, &claims.DefaultCurrency), nil
	} else if err != nil {
		l.Error("creating account", zap.Error(err), zap.Any("account", a))
		return nil, err
	}
	l.Info("created account", zap.Any("account", a))

	if err := renderToBody(w, r, views.AccountOOB(a)); err != nil {
		l.Error("rendering account oob", zap.Error(err))
		return nil, err
	}
	l.Debug("rendered account oob")

	if err := h.getAndRenderBalancesOOB(w, r); err != nil {
		return nil, err
	}

	fd := h.mapAccToFD(defaultAccount(claims, knds), &errs.ValidationError{}, knds)
	return views.CreateAccountForm(fd, knds, shownCurs, cps, &claims.DefaultCurrency), nil
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

	_, shownCurs, knds, cps, err := h.getAccDeps(r.Context())
	if err != nil {
		return nil, err
	}

	claims := getClaims(r.Context())
	fd := h.mapAccToFD(a, &errs.ValidationError{}, knds)
	return views.EditAccount(id, fd, knds, shownCurs, cps, &claims.DefaultCurrency), nil
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

	curs, shownCurs, knds, cps, err := h.getAccDeps(r.Context())
	if err != nil {
		return nil, err
	}
	a.Currency = *slicesutils.Find(curs, func(cur models.Currency) bool { return a.Currency.ID == cur.ID })
	a.Kind = *slicesutils.Find(knds, func(ak models.AccountKind) bool { return a.Kind.ID == ak.ID })

	if a.Counterparty != nil {
		if cp := slicesutils.Find(cps, func(cp models.Counterparty) bool { return a.Counterparty.ID == cp.ID }); cp != nil {
			a.Counterparty = cp
		}
	}

	if a.Kind.Enum == models.AccKindDebt && a.Counterparty != nil {
		a.Name = a.Counterparty.Name
	}

	claims := getClaims(r.Context())

	if err := a.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating account", zap.Any("errors", valerr.Errors))
			return views.EditAccount(id, h.mapAccToFD(a, &valerr, knds), knds, shownCurs, cps, &claims.DefaultCurrency), nil
		}
		l.Error("sanitizing account", zap.Error(err))
		return nil, err
	}
	l.Debug("validated account")

	err = h.repo.UpdateAccount(r.Context(), a)

	var valerr *errs.ValidationError
	if err != nil && errors.As(err, &valerr) {
		l.Error("duplication error", zap.Error(valerr), zap.Any("errors", valerr.Errors))
		return views.EditAccount(id, h.mapAccToFD(a, valerr, knds), knds, shownCurs, cps, &claims.DefaultCurrency), nil
	} else if err != nil {
		l.Error("updating account", zap.Error(err), zap.Any("account", a))
		return nil, err
	}
	l.Info("updated account", zap.Any("account", a))

	if err := h.getAndRenderBalancesOOB(w, r); err != nil {
		return nil, err
	}

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
	if err := h.repo.SetDefaultAccount(r.Context(), id); err != nil {
		l.Error("Setting as default", zap.Error(err))
		return nil, err
	}
	l.Info("Set as default", zap.String("id", id))
	return nil, nil
}

func (h Handler) AccountInputs(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	acc, err := h.mapFDToAcc(r)
	if err != nil {
		l.Error("mapping formdata to account", zap.Error(err))
		return nil, err
	}
	l.Debug("mapped formdata to account")

	_, shownCurs, knds, cps, err := h.getAccDeps(r.Context())
	if err != nil {
		l.Error("fetching account deps", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched account deps")

	id := r.FormValue("id")
	if id == "" {
		id = "0"
	}

	claims := getClaims(r.Context())
	fd := h.mapAccToFD(acc, &errs.ValidationError{}, knds)
	return views.AccountInputs(id, &fd.AccountPartialFD, knds, shownCurs, cps, &claims.DefaultCurrency), nil
}

func (h Handler) getAccDeps(ctx context.Context) ([]models.Currency, []models.Currency, []models.AccountKind, []models.Counterparty, error) {
	l := logger.FromCtx(ctx)

	curs, err := h.repo.GetAllCurrencies(ctx)
	if err != nil {
		l.Error("error fetching currencies", zap.Error(err))
		return nil, nil, nil, nil, err
	}

	shownCurs := slicesutils.Filter(curs, func(cur models.Currency) bool { return cur.IsShown })

	knds, err := h.repo.GetAccountKinds(ctx)
	if err != nil {
		l.Error("error fetching account kinds", zap.Error(err))
		return nil, nil, nil, nil, err
	}
	cps, err := h.repo.GetCounterparties(ctx)
	if err != nil {
		l.Error("error fetching counterparties", zap.Error(err))
		return nil, nil, nil, nil, err
	}
	l.Debug("fetched account dependencies")
	return curs, shownCurs, knds, cps, nil
}

func (h *Handler) mapFDToAcc(r *http.Request) (*models.Account, error) {
	var cp *models.Counterparty
	if cpId := r.FormValue("counterparty_id"); cpId != "" {
		cp = &models.Counterparty{ID: cpId}
	}

	cardLimit, _ := parseFloatIfExists(r.FormValue("card_limit"))
	expirationDate, _ := parseDateOnlyIfExists(r.FormValue("expiration_date"))

	return &models.Account{
		Name:               r.FormValue("name"),
		Kind:               models.AccountKind{ID: r.FormValue("kind_id")},
		Iban:               r.FormValue("iban"),
		CardNumber:         r.FormValue("card_number"),
		CardLimit:          cardLimit,
		NameOnCard:         r.FormValue("name_on_card"),
		Note:               r.FormValue("note"),
		SmsRef:             strings.TrimSpace(r.FormValue("sms_ref")),
		Currency:           models.Currency{ID: r.FormValue("currency_id")},
		Counterparty:       cp,
		IsArchive:          parseBoolean(r.FormValue("is_archive")),
		ExcludeFromBalance: parseBoolean(r.FormValue("exclude_from_balance")),
		ExpirationDate:     expirationDate,
	}, nil
}

func (h *Handler) mapAccToFD(acc *models.Account, errs *errs.ValidationError, knds []models.AccountKind) *views.AccountFD {
	var knd *models.AccountKind

	if acc != nil {
		knd = slicesutils.Find(knds, func(k models.AccountKind) bool { return k.ID == acc.Kind.ID })
	}
	if knd == nil {
		knd = slicesutils.Find(knds, func(k models.AccountKind) bool { return k.Enum == models.DfltAccKind })
	}
	if acc == nil {
		acc = &models.Account{Kind: *knd}
	}

	var name, cc, iban, cpID, noc, exp, limit *form.FDInput

	if knd.Enum == models.AccKindBankAcc ||
		knd.Enum == models.AccKindCreditCard {
		cc = &form.FDInput{Value: acc.CardNumber, Error: errs.Errors.MsgFor("CardNumber")}
		noc = &form.FDInput{Value: acc.NameOnCard, Error: errs.Errors.MsgFor("NameOnCard")}
	}

	if knd.Enum == models.AccKindCreditCard {
		limit = &form.FDInput{Value: formatFloatIfNonZero(acc.CardLimit), Error: errs.Errors.MsgFor("CardLimit")}
		exp = &form.FDInput{Value: formatDateOnlyIfNonZero(acc.ExpirationDate), Error: errs.Errors.MsgFor("ExpirationDate")}
	}

	if knd.Enum == models.AccKindBankAcc {
		iban = &form.FDInput{Value: acc.Iban, Error: errs.Errors.MsgFor("Iban")}
	}

	if knd.Enum != models.AccKindDebt {
		name = &form.FDInput{Value: acc.Name, Error: errs.Errors.MsgFor("Name")}
	}

	if knd.Enum == models.AccKindDebt {
		cpID = &form.FDInput{Error: errs.Errors.MsgFor("CounterpartyID")}
		if acc.Counterparty != nil {
			cpID.Value = acc.Counterparty.ID
		}
	}

	fd := &views.AccountFD{
		AccountPartialFD: views.AccountPartialFD{
			Name:           name,
			KindID:         form.FDInput{Value: acc.Kind.ID, Error: errs.Errors.MsgFor("KindID")},
			CurrencyID:     form.FDInput{Value: acc.Currency.ID, Error: errs.Errors.MsgFor("CurrencyID")},
			Iban:           iban,
			CardNumber:     cc,
			CounterpartyID: cpID,
			NameOnCard:     noc,
			CardLimit:      limit,
			ExpirationDate: exp,
		},
		Note:               form.FDInput{Value: acc.Note, Error: errs.Errors.MsgFor("Note")},
		SmsRef:             form.FDInput{Value: acc.SmsRef, Error: errs.Errors.MsgFor("SmsRef")},
		IsArchive:          form.FDInput{Value: formatBooleanIfNonZero(acc.IsArchive), Error: errs.Errors.MsgFor("IsArchive")},
		ExcludeFromBalance: form.FDInput{Value: formatBooleanIfNonZero(acc.ExcludeFromBalance), Error: errs.Errors.MsgFor("IsArchive")},
	}

	return fd
}

func defaultAccount(claims *jwt.AccessClaims, ak []models.AccountKind) *models.Account {
	return &models.Account{
		Currency:           claims.DefaultCurrency,
		Kind:               *slicesutils.Find(ak, func(k models.AccountKind) bool { return k.Enum == models.DfltAccKind }),
		IsArchive:          false,
		ExcludeFromBalance: false,
	}
}
