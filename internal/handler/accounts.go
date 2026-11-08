package handler

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/numsutils"
	"github.com/omareloui/tyche/internal/resperr"
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

	deps, err := h.getAccDeps(r.Context())
	if err != nil {
		return nil, err
	}
	l.Info("fetched accounts", zap.Int("accounts_length", len(deps.accs)))
	if err := h.repo.AttachCardStatements(r.Context(), deps.accs); err != nil {
		return nil, err
	}

	claims := getClaims(r.Context())
	fd := h.mapAccToFD(defaultAccount(claims, deps.knds), &errs.ValidationError{}, deps)
	// ?acc= opens the account's details, e.g. when reloading the page.
	openID := r.URL.Query().Get(views.AccountDetailsParam)
	return views.Accounts(claims, balances, fd, deps.knds, deps.shownCurs, deps.accs, deps.cps, openID), nil
}

func (h *Handler) AccountsSummary(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	s, err := h.repo.GetAccountsSummary(r.Context(), &getClaims(r.Context()).DefaultCurrency, accountsSummaryWith(r))
	if err != nil {
		return nil, err
	}
	return views.AccountsSummary(s), nil
}

// accountsSummaryWith reads the accounts summary's options from `with`.
func accountsSummaryWith(r *http.Request) models.AccountsSummaryWith {
	var with models.AccountsSummaryWith
	for v := range strings.SplitSeq(r.URL.Query().Get("with"), ",") {
		switch strings.TrimSpace(v) {
		case "excluded":
			with.Excluded = true
		case "goals":
			with.Goals = true
		case "budget":
			with.BudgetLeft = true
		}
	}
	return with
}

// The transactions an account's details list: its last fulfilled ones, and
// its scheduled ones due within accountUpcomingDays (overdue included), the
// first accountUpcomingTxs of them.
const (
	accountRecentTxs    = 8
	accountUpcomingTxs  = 5
	accountUpcomingDays = 31
)

// AccountDetails is the account in full, in a drawer: its balance over the
// last months, what moved in and out of it, its transactions and details.
func (h *Handler) AccountDetails(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	a, err := h.repo.GetAccount(r.Context(), r.PathValue("id"))
	if err != nil {
		l.Error("fetching account", zap.Error(err))
		return nil, err
	}

	d := &views.AccountDetailsData{Account: a, Def: &getClaims(r.Context()).DefaultCurrency}
	if d.History, err = h.repo.GetAccountHistory(r.Context(), a); err != nil {
		return nil, err
	}
	if d.Stats, err = h.repo.GetAccountTxStats(r.Context(), a.ID); err != nil {
		return nil, err
	}
	if d.Recent, err = h.repo.GetAccountRecentTransactions(r.Context(), a.ID, accountRecentTxs); err != nil {
		l.Error("fetching account transactions", zap.Error(err))
		return nil, err
	}
	until := time.Now().AddDate(0, 0, accountUpcomingDays)
	if d.Upcoming, err = h.repo.GetAccountUpcomingTransactions(r.Context(), a.ID, until); err != nil {
		l.Error("fetching account upcoming transactions", zap.Error(err))
		return nil, err
	}
	d.Upcoming = d.Upcoming[:min(len(d.Upcoming), accountUpcomingTxs)]

	return views.AccountDetails(d), nil
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

	return views.Account(a, &getClaims(r.Context()).DefaultCurrency), nil
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

	deps, err := h.getAccDeps(r.Context())
	if err != nil {
		return nil, err
	}
	deps.resolveAccount(a)

	claims := getClaims(r.Context())
	renderForm := func(valerr *errs.ValidationError) templ.Component {
		return views.CreateAccountForm(h.mapAccToFD(a, valerr, deps), deps.knds, deps.shownCurs, deps.cps, &claims.DefaultCurrency)
	}

	if err := h.validateAccount(a); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating account", zap.String("id", a.ID), zap.Any("errors", valerr.Errors))
			return renderForm(&valerr), nil
		}
		l.Error("sanitizing account", zap.Error(err))
		return nil, err
	}
	l.Debug("validated account")

	if a.Gamyaa != nil {
		err = h.repo.CreateGamyaa(r.Context(), a)
	} else {
		err = h.repo.CreateAccount(r.Context(), a)
	}

	var valerr *errs.ValidationError
	if err != nil && errors.As(err, &valerr) {
		l.Error("duplication error", zap.Error(valerr), zap.Any("errors", valerr.Errors))
		return renderForm(valerr), nil
	} else if err != nil {
		l.Error("creating account", zap.Error(err), zap.String("id", a.ID))
		return nil, err
	}
	l.Info("created account", zap.String("id", a.ID))
	drawerCreated(w)
	addHXTrigger(w, views.AccountsChangedEvent)

	// Refetched for a gam'yaa's progress or a card's statement.
	if a.Gamyaa != nil || a.Cycle.IsSet() {
		if a, err = h.repo.GetAccount(r.Context(), a.ID); err != nil {
			l.Error("fetching created account", zap.Error(err))
			return nil, err
		}
	}

	if err := renderToBody(w, r, views.AccountOOB(a, &getClaims(r.Context()).DefaultCurrency)); err != nil {
		l.Error("rendering account oob", zap.Error(err))
		return nil, err
	}
	l.Debug("rendered account oob")

	if err := h.getAndRenderBalancesOOB(w, r); err != nil {
		return nil, err
	}

	fd := h.mapAccToFD(defaultAccount(claims, deps.knds), &errs.ValidationError{}, deps)
	return views.CreateAccountForm(fd, deps.knds, deps.shownCurs, deps.cps, &claims.DefaultCurrency), nil
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

	deps, err := h.getAccDeps(r.Context())
	if err != nil {
		return nil, err
	}

	claims := getClaims(r.Context())
	fd := h.mapAccToFD(a, &errs.ValidationError{}, deps)
	return views.EditAccount(id, fd, deps.knds, deps.shownCurs, deps.cps, &claims.DefaultCurrency), nil
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

	prev, err := h.repo.GetAccount(r.Context(), id)
	if err != nil {
		l.Error("fetching account", zap.Error(err))
		return nil, err
	}

	deps, err := h.getAccDeps(r.Context())
	if err != nil {
		return nil, err
	}

	// A gam'yaa's kind and schedule stay as created (its schedule lives in
	// its transactions), and other accounts can't become one.
	if prev.Kind.Enum == models.AccKindGamyaa {
		a.Kind, a.Currency = prev.Kind, prev.Currency
	} else if slicesutils.Find(deps.knds, func(k models.AccountKind) bool {
		return k.ID == a.Kind.ID && k.Enum == models.AccKindGamyaa
	}) != nil {
		a.Kind = prev.Kind
	}
	deps.resolveAccount(a)
	a.Gamyaa = prev.Gamyaa

	claims := getClaims(r.Context())

	if err := a.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating account", zap.Any("errors", valerr.Errors))
			return views.EditAccountForm(id, h.mapAccToFD(a, &valerr, deps), deps.knds, deps.shownCurs, deps.cps, &claims.DefaultCurrency), nil
		}
		l.Error("sanitizing account", zap.Error(err))
		return nil, err
	}
	l.Debug("validated account")

	err = h.repo.UpdateAccount(r.Context(), a)

	var valerr *errs.ValidationError
	if err != nil && errors.As(err, &valerr) {
		l.Error("duplication error", zap.Error(valerr), zap.Any("errors", valerr.Errors))
		return views.EditAccountForm(id, h.mapAccToFD(a, valerr, deps), deps.knds, deps.shownCurs, deps.cps, &claims.DefaultCurrency), nil
	} else if err != nil {
		l.Error("updating account", zap.Error(err), zap.String("id", a.ID))
		return nil, err
	}
	l.Info("updated account", zap.String("id", a.ID))

	return h.renderUpdatedAccount(w, r, prev)
}

// renderUpdatedAccount responds with the refetched account (with its
// balances) in place of prev's card, or moves it to its new section of the
// list, e.g. when archived or its kind changed.
func (h *Handler) renderUpdatedAccount(w http.ResponseWriter, r *http.Request, prev *models.Account) (templ.Component, error) {
	l := logger.FromCtx(r.Context())
	addHXTrigger(w, views.AccountsChangedEvent)

	a, err := h.repo.GetAccount(r.Context(), prev.ID)
	if err != nil {
		l.Error("fetching updated account", zap.Error(err))
		return nil, err
	}

	// Before anything is written, as headers go out with the body.
	moved := views.AccountSectionChanged(prev, a)
	if moved {
		drawerSaved(w, "#"+views.AccountWrapperID(a.ID), "none")
	} else {
		drawerSaved(w, "#"+views.AccountWrapperID(a.ID), "innerHTML")
	}

	if err := h.getAndRenderBalancesOOB(w, r); err != nil {
		return nil, err
	}

	if !moved {
		return views.Account(a, &getClaims(r.Context()).DefaultCurrency), nil
	}

	if err := renderToBody(w, r, views.AccountDeleteOOB(a.ID)); err != nil {
		l.Error("rendering account delete oob", zap.Error(err))
		return nil, err
	}
	if err := renderToBody(w, r, views.AccountOOB(a, &getClaims(r.Context()).DefaultCurrency)); err != nil {
		l.Error("rendering account oob", zap.Error(err))
		return nil, err
	}
	return nil, nil
}

// ToggleArchiveAccount archives the account, or unarchives an archived one.
func (h *Handler) ToggleArchiveAccount(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
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

	prev := *a
	a.IsArchive = !a.IsArchive
	if err := h.repo.UpdateAccount(r.Context(), a); err != nil {
		l.Error("toggling account archive", zap.Error(err))
		return nil, err
	}
	l.Info("toggled account archive", zap.String("id", id), zap.Bool("is_archive", a.IsArchive))

	return h.renderUpdatedAccount(w, r, &prev)
}

var errAccountNotFound = resperr.New(http.StatusNotFound, "Account not found")

// LiveAccount guards next, a route of the account {id}: goals' and deleted
// accounts are only touched by their own actions, so they're not found.
func (h *Handler) LiveAccount(next HandlerMethod) HandlerMethod {
	return func(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
		a, err := h.repo.GetAccount(r.Context(), r.PathValue("id"))
		if err != nil || a.IsGoal() || a.IsDeleted() {
			return nil, errAccountNotFound
		}
		return next(w, r)
	}
}

// moveBalanceAccounts are the accounts a deleted account's balance can move
// to.
func moveBalanceAccounts(accs []models.Account, from string) []models.Account {
	return slicesutils.Filter(accs, func(a models.Account) bool {
		return a.ID != from && !a.IsArchive && a.Kind.Enum != models.AccKindDebt && a.Kind.Enum != models.AccKindGamyaa
	})
}

// accountAndMoveTargets is the account {id} with the accounts its balance
// can move to.
func (h *Handler) accountAndMoveTargets(r *http.Request) (*models.Account, []models.Account, error) {
	a, err := h.repo.GetAccount(r.Context(), r.PathValue("id"))
	if err != nil {
		return nil, nil, err
	}
	accs, err := h.repo.GetAccounts(r.Context())
	if err != nil {
		return nil, nil, err
	}
	return a, moveBalanceAccounts(accs, a.ID), nil
}

// GetAccountDeleteForm is the drawer deleting an account with money in it,
// moving the balance to another account or leaving it out of the totals.
func (h *Handler) GetAccountDeleteForm(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	a, accs, err := h.accountAndMoveTargets(r)
	if err != nil {
		return nil, err
	}
	fd := &views.AccountDeleteFD{}
	if t := defaultGoalAccount(accs); t != nil {
		fd.AccountID.Value = t.ID
		fd.ToAmount.Value = convertedAmount(a, t)
	}
	return views.AccountDeleteForm(a, fd, accs), nil
}

// AccountDeleteInputs renders the delete form's target again for the picked
// account (so currency).
func (h *Handler) AccountDeleteInputs(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	a, accs, err := h.accountAndMoveTargets(r)
	if err != nil {
		return nil, err
	}
	fd := &views.AccountDeleteFD{AccountID: form.FDInput{Value: r.FormValue("account_id")}}
	if t := slicesutils.Find(accs, func(t models.Account) bool { return t.ID == fd.AccountID.Value }); t != nil {
		fd.ToAmount.Value = convertedAmount(a, t)
	}
	return views.AccountDeleteInputs(a, fd, accs), nil
}

// convertedAmount is a's balance in t's currency, for a form ("" without a
// rate or when it's the same currency).
func convertedAmount(a, t *models.Account) string {
	if a.Currency.String == t.Currency.String {
		return ""
	}
	rate := a.Currency.FxRate(t.Currency.String)
	if rate == 0 {
		return ""
	}
	return formatFloatIfNonZero(numsutils.Round(a.Amount * rate))
}

// DeleteAccount soft deletes the account: it's hidden but its transactions
// stay. Its balance moves first to the picked account, if any.
func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())
	a, accs, err := h.accountAndMoveTargets(r)
	if err != nil {
		return nil, err
	}

	var move *models.Transaction
	if toID := r.FormValue("account_id"); toID != "" && a.Amount > 0 {
		fd := &views.AccountDeleteFD{
			AccountID: form.FDInput{Value: toID},
			ToAmount:  form.FDInput{Value: r.FormValue("to_amount")},
		}
		t := slicesutils.Find(accs, func(t models.Account) bool { return t.ID == toID })
		if t == nil {
			fd.AccountID.Error = "This field is required"
			return views.AccountDeleteFormFields(a, fd, accs), nil
		}
		toAmount := a.Amount
		if t.Currency.String != a.Currency.String {
			if toAmount, err = parseFloatIfExists(fd.ToAmount.Value); err != nil || toAmount <= 0 {
				fd.ToAmount.Error = "Value has to be more than 0"
				return views.AccountDeleteFormFields(a, fd, accs), nil
			}
		}
		now := time.Now()
		move = &models.Transaction{
			ID: idgen.New(), Kind: models.TransferTxKind,
			Account: *a, ToAccount: t, Amount: a.Amount, ToAmount: toAmount,
			Description: "Balance of " + a.Name + " (deleted)",
			ScheduledTo: now, FulfilledAt: now, CreatedAt: now,
		}
	}

	if err := h.repo.DeleteAccount(r.Context(), a.ID, move); err != nil {
		l.Error("deleting account", zap.Error(err))
		return nil, err
	}
	l.Info("deleted account", zap.String("id", a.ID))
	addHXTrigger(w, views.AccountsChangedEvent)

	if r.Method == http.MethodPost {
		// From the delete drawer.
		drawerSaved(w, "#"+views.AccountWrapperID(a.ID), "outerHTML")
	}
	if move != nil {
		// The target's card shows its new balance.
		t, err := h.repo.GetAccount(r.Context(), move.ToAccount.ID)
		if err != nil {
			return nil, err
		}
		if err := renderToBody(w, r, views.AccountReplaceOOB(t, &getClaims(r.Context()).DefaultCurrency)); err != nil {
			return nil, err
		}
	}
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

	accs, err := h.repo.GetAccounts(r.Context())
	if err != nil {
		l.Error("fetching accounts", zap.Error(err))
		return nil, err
	}
	prevDefault := slicesutils.Find(accs, func(a models.Account) bool { return a.IsDefault })

	if err := h.repo.SetDefaultAccount(r.Context(), id); err != nil {
		l.Error("Setting as default", zap.Error(err))
		return nil, err
	}
	l.Info("Set as default", zap.String("id", id))

	if prevDefault != nil && prevDefault.ID != id {
		prevDefault.IsDefault = false
		prev := []models.Account{*prevDefault}
		if err := h.repo.AttachCardStatements(r.Context(), prev); err != nil {
			return nil, err
		}
		prevDefault = &prev[0]
		if err := renderToBody(w, r, views.AccountReplaceOOB(prevDefault, &getClaims(r.Context()).DefaultCurrency)); err != nil {
			l.Error("rendering previous default account oob", zap.Error(err))
			return nil, err
		}
	}

	a, err := h.repo.GetAccount(r.Context(), id)
	if err != nil {
		l.Error("fetching account", zap.Error(err))
		return nil, err
	}
	return views.Account(a, &getClaims(r.Context()).DefaultCurrency), nil
}

func (h Handler) AccountInputs(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	acc, err := h.mapFDToAcc(r)
	if err != nil {
		l.Error("mapping formdata to account", zap.Error(err))
		return nil, err
	}
	l.Debug("mapped formdata to account")

	deps, err := h.getAccDeps(r.Context())
	if err != nil {
		l.Error("fetching account deps", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched account deps")

	id := r.PathValue("id")
	if id == "" {
		id = "0"
	} else if prev, err := h.repo.GetAccount(r.Context(), id); err == nil && prev.Kind.Enum == models.AccKindGamyaa {
		acc.Kind, acc.Gamyaa = prev.Kind, prev.Gamyaa
	}
	deps.resolveAccount(acc)

	claims := getClaims(r.Context())
	fd := h.mapAccToFD(acc, &errs.ValidationError{}, deps)
	return views.AccountInputs(id, &fd.AccountPartialFD, deps.knds, deps.shownCurs, deps.cps, &claims.DefaultCurrency), nil
}

type accDeps struct {
	curs, shownCurs []models.Currency
	knds            []models.AccountKind
	cps             []models.Counterparty
	accs            []models.Account
}

func (h Handler) getAccDeps(ctx context.Context) (*accDeps, error) {
	l := logger.FromCtx(ctx)

	curs, err := h.repo.GetAllCurrencies(ctx)
	if err != nil {
		l.Error("error fetching currencies", zap.Error(err))
		return nil, err
	}

	knds, err := h.repo.GetAccountKinds(ctx)
	if err != nil {
		l.Error("error fetching account kinds", zap.Error(err))
		return nil, err
	}
	cps, err := h.repo.GetCounterparties(ctx)
	if err != nil {
		l.Error("error fetching counterparties", zap.Error(err))
		return nil, err
	}
	accs, err := h.repo.GetAccounts(ctx)
	if err != nil {
		l.Error("error fetching accounts", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched account dependencies")
	return &accDeps{
		curs:      curs,
		shownCurs: slicesutils.Filter(curs, func(cur models.Currency) bool { return cur.IsShown }),
		knds:      knds,
		cps:       cps,
		accs:      accs,
	}, nil
}

// gamyaaPayFromAccounts are the accounts a gam'yaa can be paid from.
func (d *accDeps) gamyaaPayFromAccounts() []models.Account {
	return slicesutils.Filter(d.accs, func(a models.Account) bool {
		return !a.IsArchive && a.Kind.Enum != models.AccKindDebt && a.Kind.Enum != models.AccKindGamyaa
	})
}

// resolveAccount fills the account's currency, kind, counterparty and
// gam'yaa pay-from account from their IDs, and its name if it takes it from
// its counterparty.
func (d *accDeps) resolveAccount(a *models.Account) {
	if k := slicesutils.Find(d.knds, func(k models.AccountKind) bool { return k.ID == a.Kind.ID }); k != nil {
		a.Kind = *k
	}

	if a.Kind.Enum != models.AccKindCreditCard {
		a.Cycle = models.CardCycle{}
	}

	if a.Kind.Enum != models.AccKindGamyaa {
		a.Gamyaa = nil
	} else if g := a.Gamyaa; g != nil && g.PayFrom != nil {
		if pf := slicesutils.Find(d.gamyaaPayFromAccounts(), func(acc models.Account) bool { return acc.ID == g.PayFrom.ID }); pf != nil {
			g.PayFrom = pf
			// Paid in the pay-from account's currency.
			a.Currency = pf.Currency
		} else {
			g.PayFrom = nil
		}
	}

	if cur := slicesutils.Find(d.curs, func(cur models.Currency) bool { return cur.ID == a.Currency.ID }); cur != nil {
		a.Currency = *cur
	}

	if a.Counterparty != nil {
		if cp := slicesutils.Find(d.cps, func(cp models.Counterparty) bool { return cp.ID == a.Counterparty.ID }); cp != nil {
			a.Counterparty = cp
		}
	}

	if a.Kind.Enum == models.AccKindDebt && a.Counterparty != nil {
		a.Name = a.Counterparty.Name
	}
}

// validateAccount validates the account, and its gam'yaa schedule being
// created, merging their errors.
func (h *Handler) validateAccount(a *models.Account) error {
	valerr := errs.ValidationError{Errors: errs.ValidationErrors{}}
	for _, err := range []error{
		a.SanitizeAndValidate(h.sani, h.valid),
		func() error {
			if a.Gamyaa == nil {
				return nil
			}
			return a.Gamyaa.SanitizeAndValidate(h.sani, h.valid)
		}(),
	} {
		var ve errs.ValidationError
		if errors.As(err, &ve) {
			maps.Copy(valerr.Errors, ve.Errors)
		} else if err != nil {
			return err
		}
	}
	if len(valerr.Errors) > 0 {
		return valerr
	}
	return nil
}

// The due date modes of a credit card's statement (form's due_mode).
const (
	dueModeDays = "days"
	dueModeDay  = "day"
)

func (h *Handler) mapFDToAcc(r *http.Request) (*models.Account, error) {
	var cp *models.Counterparty
	if cpId := r.FormValue("counterparty_id"); cpId != "" {
		cp = &models.Counterparty{ID: cpId}
	}

	cardLimit, _ := parseFloatIfExists(r.FormValue("card_limit"))
	var cycle models.CardCycle
	cycle.StatementDay, _ = parseIntIfExists(r.FormValue("statement_day"))
	dueValue, _ := parseIntIfExists(r.FormValue("due_value"))
	if r.FormValue("due_mode") == dueModeDay {
		cycle.DueDay = dueValue
	} else {
		cycle.DueDays = dueValue
	}
	expirationDate := parseCardExpiry(r.FormValue("expiration_date"))

	// Dropped by resolveAccount unless it's a gam'yaa.
	g := &models.Gamyaa{Shares: 1}
	g.Installment, _ = parseFloatIfExists(r.FormValue("gamyaa_installment"))
	g.Members, _ = parseIntIfExists(r.FormValue("gamyaa_members"))
	if r.Form.Has("gamyaa_shares") {
		g.Shares, _ = parseFloatIfExists(r.FormValue("gamyaa_shares"))
	}
	for _, v := range r.Form["gamyaa_turns"] {
		if t, err := parseIntIfExists(v); err == nil && t > 0 && !slices.Contains(g.Turns, t) {
			g.Turns = append(g.Turns, t)
		}
	}
	slices.Sort(g.Turns)
	g.StartsOn, _ = parseDateOnlyIfExists(r.FormValue("gamyaa_starts_on"))
	if id := r.FormValue("gamyaa_pay_from_id"); id != "" {
		g.PayFrom = &models.Account{ID: id}
	}

	return &models.Account{
		Name:               r.FormValue("name"),
		Kind:               models.AccountKind{ID: r.FormValue("kind_id")},
		Iban:               r.FormValue("iban"),
		CardNumber:         r.FormValue("card_number"),
		CardLimit:          cardLimit,
		NameOnCard:         r.FormValue("name_on_card"),
		Note:               r.FormValue("note"),
		SmsRefs:            parseAliases(r.FormValue("sms_refs")),
		Currency:           models.Currency{ID: r.FormValue("currency_id")},
		Counterparty:       cp,
		IsArchive:          parseBoolean(r.FormValue("is_archive")),
		ExcludeFromBalance: parseBoolean(r.FormValue("exclude_from_balance")),
		ExpirationDate:     expirationDate,
		Gamyaa:             g,
		Cycle:              cycle,
	}, nil
}

func (h *Handler) mapAccToFD(acc *models.Account, errs *errs.ValidationError, deps *accDeps) *views.AccountFD {
	knds := deps.knds
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

	var cycle *views.CardCycleFD
	if knd.Enum == models.AccKindCreditCard {
		mode, due := dueModeDays, acc.Cycle.DueDays
		if acc.Cycle.DueDay != 0 {
			mode, due = dueModeDay, acc.Cycle.DueDay
		}
		cycle = &views.CardCycleFD{
			StatementDay: form.FDInput{Value: formatIntIfNonZero(acc.Cycle.StatementDay), Error: errs.Errors.MsgFor("StatementDay")},
			DueMode:      form.FDInput{Value: mode},
			DueValue:     form.FDInput{Value: formatIntIfNonZero(due), Error: errs.Errors.MsgFor("DueValue")},
		}
		limit = &form.FDInput{Value: formatFloatIfNonZero(acc.CardLimit), Error: errs.Errors.MsgFor("CardLimit")}
		exp = &form.FDInput{Value: formatCardExpiry(acc.ExpirationDate), Error: errs.Errors.MsgFor("ExpirationDate")}
	}

	if knd.Enum == models.AccKindBankAcc {
		iban = &form.FDInput{Value: acc.Iban, Error: errs.Errors.MsgFor("Iban")}
	}

	if knd.Enum != models.AccKindDebt {
		name = &form.FDInput{Value: acc.Name, Error: errs.Errors.MsgFor("Name")}
	}

	if knd.Enum == models.AccKindDebt || knd.Enum == models.AccKindGamyaa {
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
			Cycle:          cycle,
		},
		Note:               form.FDInput{Value: acc.Note, Error: errs.Errors.MsgFor("Note")},
		SmsRefs:            form.FDInput{Value: strings.Join(acc.SmsRefs, ", "), Error: errs.Errors.MsgFor("SmsRefs")},
		IsArchive:          form.FDInput{Value: formatBooleanIfNonZero(acc.IsArchive), Error: errs.Errors.MsgFor("IsArchive")},
		ExcludeFromBalance: form.FDInput{Value: formatBooleanIfNonZero(acc.ExcludeFromBalance), Error: errs.Errors.MsgFor("ExcludeFromBalance")},
	}

	if knd.Enum == models.AccKindGamyaa {
		fd.AccountPartialFD.Gamyaa = mapGamyaaToFD(acc, errs, deps)
	}

	return fd
}

func mapGamyaaToFD(acc *models.Account, errs *errs.ValidationError, deps *accDeps) *views.GamyaaFD {
	g := acc.Gamyaa
	if g == nil {
		g = &models.Gamyaa{Shares: 1}
	}
	payFrom := deps.gamyaaPayFromAccounts()

	if g.PayFrom == nil && len(payFrom) > 0 {
		def := slicesutils.Find(payFrom, func(a models.Account) bool { return a.IsDefault })
		if def == nil {
			def = &payFrom[0]
		}
		g.PayFrom = def
		acc.Currency = def.Currency
	}
	if g.StartsOn.IsZero() {
		g.StartsOn = time.Now().Truncate(24 * time.Hour)
	}

	turns := make([]string, len(g.Turns))
	for i, t := range g.Turns {
		turns[i] = strconv.Itoa(t)
	}
	payFromID := ""
	if g.PayFrom != nil {
		payFromID = g.PayFrom.ID
	}

	return &views.GamyaaFD{
		Installment:     form.FDInput{Value: formatFloatIfNonZero(g.Installment), Error: errs.Errors.MsgFor("Installment")},
		Members:         form.FDInput{Value: formatIntIfNonZero(g.Members), Error: errs.Errors.MsgFor("Members")},
		Shares:          form.FDInput{Value: formatFloatIfNonZero(g.Shares), Error: errs.Errors.MsgFor("Shares")},
		Turns:           form.FDInput{Value: strings.Join(turns, ","), Error: errs.Errors.MsgFor("Turns")},
		StartsOn:        form.FDInput{Value: formatDateOnlyIfNonZero(g.StartsOn), Error: errs.Errors.MsgFor("StartsOn")},
		PayFromID:       form.FDInput{Value: payFromID, Error: errs.Errors.MsgFor("PayFromID")},
		PayFromAccounts: payFrom,
		Gamyaa:          g,
		Currency:        acc.Currency,
	}
}

func defaultAccount(claims *jwt.AccessClaims, ak []models.AccountKind) *models.Account {
	return &models.Account{
		Currency:           claims.DefaultCurrency,
		Kind:               *slicesutils.Find(ak, func(k models.AccountKind) bool { return k.Enum == models.DfltAccKind }),
		IsArchive:          false,
		ExcludeFromBalance: false,
	}
}

// parseCardExpiry parses a card's "MM/YY" expiry (or a yyyy-mm-dd date) as
// the first day of that month, zero if empty or invalid.
func parseCardExpiry(str string) time.Time {
	if t, err := time.Parse("01/06", strings.TrimSpace(str)); err == nil {
		return t
	}
	t, _ := parseDateOnlyIfExists(str)
	return t
}

func formatCardExpiry(date time.Time) string {
	if date.IsZero() {
		return ""
	}
	return date.Format("01/06")
}

// GetFixBalances is the drawer stepping through the active accounts with
// their balance editable, to set them to what they really are.
func (h *Handler) GetFixBalances(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	if r.Header.Get("HX-Request") == "" {
		// The old page's url: the drawer opens from the accounts page.
		http.Redirect(w, r, "/accounts", http.StatusSeeOther)
		return nil, nil
	}
	accs, err := h.activeAccounts(r.Context())
	if err != nil {
		return nil, err
	}
	return views.FixBalancesDrawer(accs, views.NewFixBalancesFD(accs)), nil
}

// FixBalances sets the submitted balances (as the banks show them) as
// adjustment transactions of the differences, answering the changed cards
// out of band.
func (h *Handler) FixBalances(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	if err := r.ParseForm(); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "invalid form")
	}

	accs, err := h.activeAccounts(r.Context())
	if err != nil {
		return nil, err
	}

	fd := &views.FixBalancesFD{Balances: map[string]*views.BalanceFD{}, Changed: -1}
	var fixes []models.BalanceFix
	for i := range accs {
		a := &accs[i]
		bfd := balanceFD(r, a, "-"+a.ID)
		fd.Balances[a.ID] = bfd
		if bfd.Amount.Value == "" {
			continue
		}
		fix, err := parseBalanceFix(a, bfd)
		if err != nil {
			bfd.Amount.Error = "Invalid balance"
			fd.Error = "Invalid balance for " + a.Name + "."
			continue
		}
		fixes = append(fixes, fix)
	}
	if fd.Error != "" {
		return views.FixBalancesForm(accs, fd), nil
	}

	changed, err := h.repo.FixBalances(r.Context(), fixes, time.Now())
	if err != nil {
		return nil, err
	}
	l.Info("fixed balances", zap.Int("changed", changed))
	if changed == 0 {
		fd.Changed = 0
		return views.FixBalancesForm(accs, fd), nil
	}

	addHXTrigger(w, views.AccountsChangedEvent)
	drawerSaved(w, "", "")
	w.Header().Set("HX-Reswap", "none")

	now, err := h.activeAccounts(r.Context())
	if err != nil {
		return nil, err
	}
	def := &getClaims(r.Context()).DefaultCurrency
	for i := range now {
		prev := slicesutils.Find(accs, func(a models.Account) bool { return a.ID == now[i].ID })
		if prev != nil && prev.Amount == now[i].Amount {
			continue
		}
		if err := renderToBody(w, r, views.AccountReplaceOOB(&now[i], def)); err != nil {
			return nil, err
		}
	}
	if err := h.getAndRenderBalancesOOB(w, r); err != nil {
		return nil, err
	}
	return nil, nil
}

// GetAdjustBalance is the drawer setting the account's balance.
func (h *Handler) GetAdjustBalance(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	a, err := h.adjustableAccount(r)
	if err != nil {
		return nil, err
	}
	return views.AdjustBalanceDrawer(a, views.NewBalanceFD(a)), nil
}

// AdjustBalance sets the account's balance (as the bank shows it) as an
// adjustment transaction of the difference, answering its card.
func (h *Handler) AdjustBalance(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())
	a, err := h.adjustableAccount(r)
	if err != nil {
		return nil, err
	}

	fd := balanceFD(r, a, "")
	fix, err := parseBalanceFix(a, fd)
	if fd.Amount.Value == "" || err != nil {
		fd.Amount.Error = "Invalid balance"
		return views.AdjustBalanceForm(a, fd), nil
	}

	changed, err := h.repo.FixBalances(r.Context(), []models.BalanceFix{fix}, time.Now())
	if err != nil {
		return nil, err
	}
	l.Info("adjusted balance", zap.String("id", a.ID), zap.Int("changed", changed))
	if changed == 0 {
		drawerSaved(w, "", "")
		w.Header().Set("HX-Reswap", "none")
		return nil, nil
	}
	return h.renderUpdatedAccount(w, r, a)
}

// balanceFD is a's typed balance, its fields named amount<suffix> and
// amount_unit<suffix>.
func balanceFD(r *http.Request, a *models.Account, suffix string) *views.BalanceFD {
	v := strings.TrimSpace(r.PostFormValue("amount" + suffix))
	if !strings.ContainsAny(strings.TrimPrefix(v, a.Currency.Symbol), "0123456789") {
		// Nothing typed but the mask's symbol.
		v = ""
	}
	return &views.BalanceFD{
		Amount: form.FDInput{Value: v},
		Unit:   string(a.AmountUnit(r.PostFormValue("amount_unit" + suffix))),
	}
}

// parseBalanceFix reads fd, a's balance as the bank shows it (e.g.
// "EGP -1,250.50", what's owed for a credit card).
func parseBalanceFix(a *models.Account, fd *views.BalanceFD) (models.BalanceFix, error) {
	v := strings.TrimSpace(strings.TrimPrefix(fd.Amount.Value, a.Currency.Symbol))
	n, err := parseFloatIfExists(v)
	if err != nil {
		return models.BalanceFix{}, err
	}
	if strings.ContainsAny(v, "-−") {
		n = -n
	}
	return a.FixOfBankEntry(models.FromUnit(n, models.ParseMetalUnit(fd.Unit))), nil
}

// adjustableAccount is the account {id}, if its balance can be adjusted
// (not archived).
func (h *Handler) adjustableAccount(r *http.Request) (*models.Account, error) {
	a, err := h.repo.GetAccount(r.Context(), r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	if a.IsArchive {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Unarchive the account to adjust its balance.")
	}
	return a, nil
}

func (h *Handler) activeAccounts(ctx context.Context) ([]models.Account, error) {
	accs, err := h.repo.GetAccounts(ctx)
	if err != nil {
		return nil, err
	}
	return slicesutils.Filter(accs, func(a models.Account) bool { return !a.IsArchive }), nil
}
