package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

var linkBoughtForErrs = []error{
	models.ErrBoughtForKinds,
	models.ErrBoughtForExpense,
	models.ErrBoughtForCurrency,
	models.ErrBoughtForExceeds,
	models.ErrBoughtForLinked,
}

// boughtByFD is the create form's "someone bought it for me" as sent.
func boughtByFD(r *http.Request) views.BoughtByFD {
	return views.BoughtByFD{
		On:        form.FDInput{Value: r.FormValue("bought_by_on")},
		AccountID: form.FDInput{Value: r.FormValue("bought_by")},
	}
}

// boughtByDebt is the debt borrowed from the person the create form says
// bought the expense for the user (nil when it doesn't), out of their Debt
// account into the expense's, the same day. The returned validation error is
// the form's, on BoughtBy.
func boughtByDebt(r *http.Request, expense *models.Transaction, accs []models.Account) (*models.Transaction, *errs.ValidationError) {
	if r.FormValue("bought_by_on") == "" || expense.Kind != models.ExpenseTxKind {
		return nil, nil
	}
	fieldErr := func(tag string) *errs.ValidationError {
		return &errs.ValidationError{Errors: errs.ValidationErrors{"BoughtBy": {Tag: tag}}}
	}
	if expense.RecurrentTx != nil {
		return nil, fieldErr("bought_by_once")
	}
	id := r.FormValue("bought_by")
	from := slicesutils.Find(accs, func(a models.Account) bool {
		return a.ID == id && a.Kind.Enum == models.AccKindDebt && !a.IsArchive
	})
	if from == nil {
		return nil, fieldErr("bought_by_person")
	}

	amount := expense.Amount
	if from.Currency.ID != expense.Account.Currency.ID {
		if rate, err := fxrate.GetFxRate(expense.Account.Currency.String, from.Currency.String); err == nil && rate > 0 {
			amount = from.Currency.Round(expense.Amount * rate)
		}
	}
	to := expense.Account
	return &models.Transaction{
		ID:          idgen.New(),
		Kind:        models.DebtTxKind,
		Account:     *from,
		ToAccount:   &to,
		Amount:      amount,
		ToAmount:    expense.Amount,
		Description: boughtByDescription(expense),
		ScheduledTo: expense.ScheduledTo,
		FulfilledAt: expense.FulfilledAt,
		CreatedAt:   expense.CreatedAt,
	}, nil
}

// boughtByDescription names what a debt bought: the expense's description,
// else its counterparty.
func boughtByDescription(expense *models.Transaction) string {
	if expense.Description != "" {
		return expense.Description
	}
	if expense.Counterparty != nil {
		return expense.Counterparty.Name
	}
	return ""
}

// QuickCreateBoughtByPerson answers the "bought by" combobox's create: the
// Debt account of the person named, created (with them as a counterparty)
// in the user's main currency when they have none.
func (h Handler) QuickCreateBoughtByPerson(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	cp := dfltCounterparty()
	cp.ID = idgen.New()
	cp.Name = r.FormValue("name")
	cp.IsMerchant = false
	if err := cp.SanitizeAndValidate(h.sani, h.valid); err != nil {
		l.Error("quick create bought by person validation", zap.Error(err))
		return nil, resperr.New(http.StatusBadRequest, err.Error())
	}
	cp, err := h.findOrCreateCounterparty(r.Context(), cp)
	if err != nil {
		return nil, err
	}

	acc, err := h.debtAccountOf(r.Context(), cp)
	if err != nil {
		return nil, err
	}
	return views.BoughtByComboOption(acc), nil
}

// debtAccountOf is the person's live Debt account (in the user's main
// currency first), else one created for them in it.
func (h Handler) debtAccountOf(ctx context.Context, cp *models.Counterparty) (*models.Account, error) {
	l := logger.FromCtx(ctx)

	accs, err := h.repo.GetAccounts(ctx)
	if err != nil {
		return nil, err
	}
	def := getClaims(ctx).DefaultCurrency
	var found *models.Account
	for i := range accs {
		a := &accs[i]
		if a.Kind.Enum != models.AccKindDebt || a.Counterparty == nil || a.Counterparty.ID != cp.ID {
			continue
		}
		if found == nil || a.Currency.ID == def.ID {
			found = a
		}
	}
	if found != nil {
		if found.IsArchive {
			return nil, resperr.New(http.StatusConflict, "Their debt account is archived, unarchive it on the accounts page")
		}
		return found, nil
	}

	kinds, err := h.repo.GetAccountKinds(ctx)
	if err != nil {
		return nil, err
	}
	kind := slicesutils.Find(kinds, func(k models.AccountKind) bool { return k.Enum == models.AccKindDebt })
	if kind == nil {
		return nil, errors.New("no debt account kind")
	}

	name := cp.Name
	if slicesutils.Find(accs, func(a models.Account) bool { return strings.EqualFold(a.Name, name) }) != nil {
		name += " (" + def.String + ")"
	}
	acc := &models.Account{
		ID:           idgen.New(),
		Name:         name,
		Kind:         *kind,
		Currency:     def,
		Counterparty: cp,
	}
	if err := h.repo.CreateAccount(ctx, acc); err != nil {
		l.Error("creating bought by debt account", zap.Error(err))
		return nil, err
	}
	l.Info("quick created debt account", zap.String("id", acc.ID))
	return acc, nil
}

// GetBoughtByPicker lists the debts an expense could have been bought with.
func (h Handler) GetBoughtByPicker(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return h.renderBoughtByPicker(r, "")
}

// LinkBoughtBy links the picked debt to the expense in the path.
func (h Handler) LinkBoughtBy(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	debtID := r.FormValue("debt_id")
	if id == "" || debtID == "" {
		return nil, errs.ErrInvalidID
	}
	if msg, err := h.linkBoughtFor(r, debtID, id); err != nil || msg != "" {
		if msg != "" {
			return h.renderBoughtByPicker(r, msg)
		}
		return nil, err
	}
	return h.answerBoughtForLink(w, r)
}

// GetBoughtForPicker lists the expenses a debt could have bought.
func (h Handler) GetBoughtForPicker(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return h.renderBoughtForPicker(r, "")
}

// LinkBoughtFor links the debt in the path to the picked expense.
func (h Handler) LinkBoughtFor(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	expenseID := r.FormValue("expense_id")
	if id == "" || expenseID == "" {
		return nil, errs.ErrInvalidID
	}
	if msg, err := h.linkBoughtFor(r, id, expenseID); err != nil || msg != "" {
		if msg != "" {
			return h.renderBoughtForPicker(r, msg)
		}
		return nil, err
	}
	return h.answerBoughtForLink(w, r)
}

// UnlinkBoughtFor makes the debt in the path a plain debt again.
func (h Handler) UnlinkBoughtFor(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	if err := h.repo.UnlinkBoughtFor(r.Context(), id); err != nil {
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("unlinked bought for")
	return h.answerBoughtForLink(w, r)
}

// linkBoughtFor links debtID to expenseID, returning the refusal to show.
func (h Handler) linkBoughtFor(r *http.Request, debtID, expenseID string) (string, error) {
	l := logger.FromCtx(r.Context())
	if err := h.repo.LinkBoughtFor(r.Context(), debtID, expenseID); err != nil {
		for _, e := range linkBoughtForErrs {
			if errors.Is(err, e) {
				return e.Error(), nil
			}
		}
		l.Error("linking bought for", zap.Error(err))
		return "", err
	}
	l.Info("linked bought for")
	return "", nil
}

// answerBoughtForLink re-renders the list (the debt moves under its expense
// or back to its own day), and the open details when asked from them.
func (h Handler) answerBoughtForLink(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	if r.Header.Get("HX-Target") == txDetailsPanel {
		if err := h.renderTransactionsListOOB(w, r); err != nil {
			return nil, err
		}
		d, err := h.txDetails(r.Context(), r.FormValue("details_id"))
		if err != nil {
			return nil, err
		}
		return views.TxDetailsPanel(d, &getClaims(r.Context()).DefaultCurrency), nil
	}
	if err := h.renderTransactionsListOOB(w, r); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h Handler) renderBoughtByPicker(r *http.Request, errMsg string) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if !tx.CanBeBoughtBy() {
		return nil, resperr.New(http.StatusUnprocessableEntity, models.ErrBoughtForExpense.Error())
	}
	debts, err := h.repo.GetBoughtByCandidates(r.Context(), tx)
	if err != nil {
		return nil, err
	}
	return views.BoughtByPicker(tx, debts, errMsg), nil
}

func (h Handler) renderBoughtForPicker(r *http.Request, errMsg string) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if !tx.IsBorrow() {
		return nil, resperr.New(http.StatusUnprocessableEntity, models.ErrBoughtForKinds.Error())
	}
	exps, err := h.repo.GetBoughtForCandidates(r.Context(), tx)
	if err != nil {
		return nil, err
	}
	return views.BoughtForPicker(tx, exps, errMsg), nil
}

// CancelTxPicker answers a row's picker's Cancel (reimburses, transfer pair,
// recurrent match, bought by / for): the loaded list again, out of band, so
// the row comes back as the list shows it (nested ones, chips).
func (h Handler) CancelTxPicker(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	if r.PathValue("id") == "" {
		return nil, errs.ErrInvalidID
	}
	if err := h.renderTransactionsListOOB(w, r); err != nil {
		return nil, err
	}
	return nil, nil
}
