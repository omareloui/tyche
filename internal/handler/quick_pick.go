package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
	"go.uber.org/zap"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
)

// Quick picks: a details drawer's category / counterparty picked again in
// place (views.QuickPick). GET /<entity>/{id}/pick/{field} answers the
// opened combobox (views.QuickPicker, its options loaded on demand), its
// PUT saves the picked value alone (empty clearing it) and answers what
// shows it: the transaction's / recurrent transaction's details (what they
// show depends on it), else the pick, with the cards out of band.

var errUnknownPick = resperr.New(http.StatusNotFound, "Nothing to pick there")

// pickField is the {field} path value, one of views.QuickPick*.
func pickField(r *http.Request) (string, error) {
	switch f := r.PathValue("field"); f {
	case views.QuickPickCategory, views.QuickPickCounterparty:
		return f, nil
	}
	return "", errUnknownPick
}

// txPickField is a transaction's {field}: pickField's, its account or its
// date.
func txPickField(r *http.Request) (string, error) {
	if f := r.PathValue("field"); f == views.QuickPickAccount || f == views.QuickPickDate {
		return f, nil
	}
	return pickField(r)
}

// pickRefused shows msg in place of the pick (pickID's) rather than over
// the details.
func pickRefused(w http.ResponseWriter, pickID, msg string) error {
	w.Header().Set("HX-Retarget", "#"+pickID)
	w.Header().Set("HX-Reswap", "outerHTML")
	return resperr.New(http.StatusUnprocessableEntity, msg)
}

// pickedCategory is the category named by the form's key, nil for none,
// refused when it's not one of the user's or doesn't allow kind.
func (h Handler) pickedCategory(ctx context.Context, id string, kind models.TxKind) (*models.Category, bool, error) {
	if id == "" {
		return nil, true, nil
	}
	cat, err := h.repo.GetCategory(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return cat, slices.Contains(cat.Kinds, kind.CategoryKind()), nil
}

// pickedCounterparty is the counterparty named by id, nil for none, false
// when it's not one of the user's.
func (h Handler) pickedCounterparty(ctx context.Context, id string) (*models.Counterparty, bool, error) {
	if id == "" {
		return nil, true, nil
	}
	cp, err := h.repo.GetCounterparty(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return cp, true, nil
}

// pickableTx is the {id} transaction when the details can pick its refs.
func (h Handler) pickableTx(r *http.Request) (*models.Transaction, error) {
	tx, err := h.editableTx(r)
	if err != nil {
		return nil, err
	}
	if !views.TxRefsEditable(tx) {
		return nil, resperr.New(http.StatusUnprocessableEntity, "This transaction takes no category or counterparty here.")
	}
	return tx, nil
}

// datePickableTx is the {id} transaction when the details can move its day.
func (h Handler) datePickableTx(r *http.Request) (*models.Transaction, error) {
	tx, err := h.editableTx(r)
	if err != nil {
		return nil, err
	}
	if !views.TxDateEditable(tx) {
		return nil, resperr.New(http.StatusUnprocessableEntity, "This transaction's date can't change here.")
	}
	return tx, nil
}

func (h Handler) TxPicker(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	field, err := txPickField(r)
	if err != nil {
		return nil, err
	}
	if field == views.QuickPickDate {
		tx, err := h.datePickableTx(r)
		if err != nil {
			return nil, err
		}
		return views.TxDatePicker(tx), nil
	}
	tx, err := h.pickableTx(r)
	if err != nil {
		return nil, err
	}
	if field == views.QuickPickAccount {
		if !views.TxAccountEditable(tx) {
			return nil, resperr.New(http.StatusUnprocessableEntity, "This transaction's account can't change here.")
		}
		accs, err := h.repo.GetAccounts(r.Context())
		if err != nil {
			return nil, err
		}
		return views.TxAccountPicker(tx, accs), nil
	}
	if field == views.QuickPickCategory {
		cats, err := h.repo.GetCategories(r.Context())
		if err != nil {
			return nil, err
		}
		return views.TxCategoryPicker(tx, cats), nil
	}
	cps, err := h.repo.GetCounterparties(r.Context())
	if err != nil {
		return nil, err
	}
	return views.TxCounterpartyPicker(tx, cps), nil
}

// SetTxPick saves a transaction's picked category or counterparty (marking
// it reviewed, as editing it does), answering its details panel and the
// list out of band.
func (h Handler) SetTxPick(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	field, err := txPickField(r)
	if err != nil {
		return nil, err
	}
	pickable := h.pickableTx
	if field == views.QuickPickDate {
		pickable = h.datePickableTx
	}
	tx, err := pickable(r)
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	l := logger.FromCtx(ctx)
	pickID := views.QuickPickID("tx", tx.ID, field)

	if field == views.QuickPickAccount {
		return h.setTxAccountPick(w, r, tx, pickID)
	}

	var edit models.TxBulkEdit
	if field == views.QuickPickDate {
		day, err := time.Parse(time.DateOnly, strings.TrimSpace(r.FormValue("scheduled_to")))
		if err != nil {
			return nil, pickRefused(w, pickID, "Pick a day.")
		}
		if !tx.FulfilledAt.IsZero() && day.After(h.userToday(ctx)) {
			return nil, pickRefused(w, pickID, "A fulfilled transaction can't move ahead of today.")
		}
		edit.Date = &day
	} else if field == views.QuickPickCategory {
		cat, ok, err := h.pickedCategory(ctx, r.FormValue("category_id"), tx.Kind)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, pickRefused(w, pickID, "That category doesn't fit this transaction.")
		}
		edit.Category, edit.ClearCategory = cat, cat == nil
	} else {
		cp, ok, err := h.pickedCounterparty(ctx, r.FormValue("counterparty_id"))
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, pickRefused(w, pickID, "Counterparty not found.")
		}
		edit.Counterparty, edit.ClearCounterparty = cp, cp == nil
		// As the form fills an empty category on picking a counterparty.
		if tx.Category == nil || tx.Category.ID == "" {
			edit.Category = cp.DefaultCategoryFor(tx.Kind)
		}
	}

	out, skips := edit.Apply(tx, h.userToday(ctx))
	if out == nil || len(skips) > 0 {
		return nil, pickRefused(w, pickID, "This transaction can't take it.")
	}
	// Its lines and receipt stay as they are.
	out.Items, out.ReceiptID = nil, ""
	if err := h.repo.BulkUpdateTransactions(ctx, []*models.Transaction{tx}, []*models.Transaction{out}, models.TxBulkReviewDone); err != nil {
		l.Error("saving transaction pick", zap.Error(err), zap.String("id", tx.ID), zap.String("field", field))
		return nil, err
	}
	l.Info("picked transaction ref", zap.String("id", tx.ID), zap.String("field", field))

	if err := h.renderTransactionsListOOB(w, r); err != nil {
		return nil, err
	}
	d, err := h.txDetails(ctx, tx.ID)
	if err != nil {
		return nil, err
	}
	return views.TxDetailsPanel(d, &getClaims(ctx).DefaultCurrency), nil
}

// pickableRecurrentTx is the {id} recurrent transaction when the details
// can pick its refs.
func (h Handler) pickableRecurrentTx(r *http.Request) (*models.RecurrentTx, error) {
	rt, err := h.liveRecurrentTx(r)
	if err != nil {
		return nil, err
	}
	if !views.RecurrentRefsEditable(rt) {
		return nil, resperr.New(http.StatusUnprocessableEntity, "It takes no category or counterparty.")
	}
	return rt, nil
}

func (h Handler) RecurrentTxPicker(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	field, err := pickField(r)
	if err != nil {
		return nil, err
	}
	rt, err := h.pickableRecurrentTx(r)
	if err != nil {
		return nil, err
	}
	if field == views.QuickPickCategory {
		cats, err := h.repo.GetCategories(r.Context())
		if err != nil {
			return nil, err
		}
		return views.RecurrentCategoryPicker(rt, cats), nil
	}
	cps, err := h.repo.GetCounterparties(r.Context())
	if err != nil {
		return nil, err
	}
	return views.RecurrentCounterpartyPicker(rt, cps), nil
}

// SetRecurrentTxPick saves a recurrent transaction's picked category or
// counterparty, applied to its scheduled occurrences as an edit is,
// answering its details with the card out of band.
func (h Handler) SetRecurrentTxPick(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	field, err := pickField(r)
	if err != nil {
		return nil, err
	}
	rt, err := h.pickableRecurrentTx(r)
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	l := logger.FromCtx(ctx)
	pickID := views.QuickPickID("rtx", rt.ID, field)

	next := *rt
	// Its lines stay as they are.
	next.Items = nil
	if field == views.QuickPickCategory {
		cat, ok, err := h.pickedCategory(ctx, r.FormValue("category_id"), rt.Kind)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, pickRefused(w, pickID, "That category doesn't fit it.")
		}
		next.Category = cat
	} else {
		cp, ok, err := h.pickedCounterparty(ctx, r.FormValue("counterparty_id"))
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, pickRefused(w, pickID, "Counterparty not found.")
		}
		next.Counterparty = cp
		if rt.Category == nil || rt.Category.ID == "" {
			next.Category = cp.DefaultCategoryFor(rt.Kind)
		}
	}

	if err := h.repo.UpdateRecurrentTx(ctx, rt, &next); err != nil {
		l.Error("saving recurrent tx pick", zap.Error(err), zap.String("id", rt.ID), zap.String("field", field))
		return nil, err
	}
	l.Info("picked recurrent tx ref", zap.String("id", rt.ID), zap.String("field", field))

	if err := h.renderRecurrentTxsSummaryOOB(w, r); err != nil {
		return nil, err
	}
	saved, err := h.repo.GetRecurrentTx(ctx, rt.ID)
	if err != nil {
		return nil, err
	}
	if err := renderToBody(w, r, views.RecurrentTxOOB(saved)); err != nil {
		return nil, err
	}
	return h.RecurrentTxDetails(w, r)
}

// pickableMerchant is the {id} counterparty when it's a merchant (only they
// take a default category).
func (h Handler) pickableMerchant(r *http.Request) (*models.Counterparty, error) {
	cp, err := h.repo.GetCounterparty(r.Context(), r.PathValue("id"))
	if err != nil {
		return nil, notFoundOr(err, "Counterparty not found")
	}
	if !cp.IsMerchant {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Only merchants have a default category.")
	}
	return cp, nil
}

func (h Handler) CounterpartyCategoryPicker(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	cp, err := h.pickableMerchant(r)
	if err != nil {
		return nil, err
	}
	cats, err := h.repo.GetCategories(r.Context())
	if err != nil {
		return nil, err
	}
	return views.CounterpartyCategoryPicker(cp, cats), nil
}

// SetCounterpartyCategoryPick saves a merchant's picked default category
// (an expense one, as its form), answering the pick and its card out of
// band.
func (h Handler) SetCounterpartyCategoryPick(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	cp, err := h.pickableMerchant(r)
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	l := logger.FromCtx(ctx)
	cat, ok, err := h.pickedCategory(ctx, r.FormValue("default_category_id"), models.ExpenseTxKind)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, pickRefused(w, views.QuickPickID("cp", cp.ID, views.QuickPickCategory), "A default category is an expense one.")
	}
	catID := ""
	if cat != nil {
		catID = cat.ID
	}
	if err := h.repo.SetCounterpartyDefaultCategory(ctx, cp.ID, catID); err != nil {
		l.Error("setting counterparty default category", zap.Error(err), zap.String("id", cp.ID))
		return nil, err
	}
	l.Info("picked counterparty default category", zap.String("id", cp.ID))

	if cp, err = h.repo.GetCounterparty(ctx, cp.ID); err != nil {
		return nil, err
	}
	addHXTrigger(w, views.CounterpartiesChangedEvent)
	if err := renderToBody(w, r, views.CounterpartyReplaceOOB(cp)); err != nil {
		return nil, err
	}
	return views.CounterpartyCategoryPick(cp), nil
}

func (h Handler) ItemCategoryPicker(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	it, err := h.item(r)
	if err != nil {
		return nil, err
	}
	cats, err := h.itemCategories(r)
	if err != nil {
		return nil, err
	}
	return views.ItemCategoryPicker(&it.Item, cats), nil
}

// SetItemCategoryPick saves an item's picked category, answering the pick;
// the items list re-renders on items-changed.
func (h Handler) SetItemCategoryPick(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	it, err := h.item(r)
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	l := logger.FromCtx(ctx)
	catID := r.FormValue("category_id")
	if catID != "" {
		cats, err := h.itemCategories(r)
		if err != nil {
			return nil, err
		}
		if !slices.ContainsFunc(cats, func(c models.ItemCategory) bool { return c.ID == catID }) {
			return nil, pickRefused(w, views.QuickPickID("item", it.Item.ID, views.QuickPickCategory), "Category not found.")
		}
	}
	if err := h.repo.SetItemCategory(ctx, it.Item.ID, catID); err != nil {
		l.Error("setting item category", zap.Error(err), zap.String("id", it.Item.ID))
		return nil, err
	}
	l.Info("picked item category", zap.String("id", it.Item.ID))

	if it, err = h.item(r); err != nil {
		return nil, err
	}
	addHXTrigger(w, views.ItemsChangedEvent)
	return views.ItemCategoryPick(&it.Item), nil
}

// setTxAccountPick moves the transaction to the picked account, one of the
// same currency it can be in (views.TxAccountOptions), as editing it does:
// its balance effect moves too. Answers the details, the list and the
// balances out of band.
func (h Handler) setTxAccountPick(w http.ResponseWriter, r *http.Request, tx *models.Transaction, pickID string) (templ.Component, error) {
	ctx := r.Context()
	l := logger.FromCtx(ctx)
	if !views.TxAccountEditable(tx) {
		return nil, pickRefused(w, pickID, "This transaction's account can't change here.")
	}
	accs, err := h.repo.GetAccounts(ctx)
	if err != nil {
		return nil, err
	}
	id := r.FormValue("account_id")
	i := slices.IndexFunc(views.TxAccountOptions(tx, accs), func(a models.Account) bool { return a.ID == id })
	if i < 0 {
		return nil, pickRefused(w, pickID, "Pick an account in "+tx.Account.Currency.String+".")
	}
	acc := views.TxAccountOptions(tx, accs)[i]

	if acc.ID != tx.Account.ID {
		out := *tx
		out.Account = acc
		// Its lines, tags and receipt stay as they are.
		out.Items, out.Tags, out.ReceiptID = nil, nil, ""
		if err := h.repo.UpdateTransaction(ctx, tx, &out); err != nil {
			l.Error("moving transaction to an account", zap.Error(err), zap.String("id", tx.ID))
			return nil, err
		}
		l.Info("picked transaction account", zap.String("id", tx.ID))
	}

	if err := h.renderTransactionsListOOB(w, r); err != nil {
		return nil, err
	}
	if !tx.FulfilledAt.IsZero() {
		if err := h.getAndRenderBalancesOOB(w, r); err != nil {
			return nil, err
		}
	}
	d, err := h.txDetails(ctx, tx.ID)
	if err != nil {
		return nil, err
	}
	return views.TxDetailsPanel(d, &getClaims(ctx).DefaultCurrency), nil
}
