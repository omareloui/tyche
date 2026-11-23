package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"go.uber.org/zap"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
)

// Return + warranty windows of a purchase (models/return_warranty.go), set
// from its details in place: the purchase's (PUT /transactions/{id}/windows)
// and an item line's warranty (PUT /transactions/{id}/items/{itemID}/warranty).

var errNoWindows = resperr.New(http.StatusUnprocessableEntity, "Only purchases of your own can have a return window or a warranty.")

// SetTransactionWindows sets the purchase's return window (`return`), its
// whole warranty (`warranty`), each a preset ("14d", "12m"), a day or ""
// (none), and whether it was returned (`returned` 1 / 0), only the sent
// ones changing. Answers the section, the row out of band.
func (h Handler) SetTransactionWindows(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	tx, err := h.editableTx(r)
	if err != nil {
		return nil, err
	}
	if !models.TxWindowsEditable(tx) {
		return nil, errNoWindows
	}
	if err := r.ParseForm(); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "Invalid form")
	}
	ret, war, returned := tx.ReturnUntil, tx.WarrantyUntil, tx.ReturnedAt
	p := views.TxWindowsProps{Tx: tx}
	if r.Form.Has("return") {
		if ret, err = models.ParseWindowValue(r.FormValue("return"), tx.ScheduledTo); err != nil {
			p.Error = "Pick a valid day to return it by."
			return views.TxWindows(p), nil
		}
	}
	if r.Form.Has("warranty") {
		if war, err = models.ParseWindowValue(r.FormValue("warranty"), tx.ScheduledTo); err != nil {
			p.Error = "Pick a valid day for the warranty."
			return views.TxWindows(p), nil
		}
	}
	if r.Form.Has("returned") {
		returned = nil
		if r.FormValue("returned") == "1" {
			today := h.userToday(r.Context())
			returned = &today
		}
	}
	if (ret != nil && ret.Before(dayOf(tx.ScheduledTo))) || (war != nil && war.Before(dayOf(tx.ScheduledTo))) {
		p.Error = "It can't end before the purchase."
		return views.TxWindows(p), nil
	}

	l := logger.FromCtx(r.Context())
	if err := h.repo.SetTransactionWindows(r.Context(), tx.ID, ret, war, returned); err != nil {
		return nil, err
	}
	l.Info("set transaction windows", zap.String("id", tx.ID))
	tx.ReturnUntil, tx.WarrantyUntil, tx.ReturnedAt = ret, war, returned
	p.RefundID, err = h.repo.ReturnRefundCandidate(r.Context(), tx)
	if err != nil {
		return nil, err
	}
	if err := h.renderTransactionsListOOB(w, r); err != nil {
		return nil, err
	}
	return views.TxWindows(p), nil
}

// SetTxItemWarranty sets an item line's own warranty (`warranty`, as
// SetTransactionWindows'), answering the details' lines.
func (h Handler) SetTxItemWarranty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	tx, err := h.editableTx(r)
	if err != nil {
		return nil, err
	}
	if !models.TxWindowsEditable(tx) {
		return nil, errNoWindows
	}
	d, err := models.ParseWindowValue(r.FormValue("warranty"), tx.ScheduledTo)
	if err != nil || (d != nil && d.Before(dayOf(tx.ScheduledTo))) {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Pick a valid day for the warranty.")
	}
	err = h.repo.SetTxItemWarranty(r.Context(), tx.ID, r.PathValue("itemID"), d)
	if errors.Is(err, repository.ErrItemNotFound) {
		return nil, resperr.New(http.StatusNotFound, "Item not found")
	}
	if err != nil {
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("set item warranty", zap.String("id", r.PathValue("itemID")))
	return h.txItemsAnswer(w, r, tx.ID, nil, nil)
}

// dayOf is t's day at midnight UTC, as the windows are.
func dayOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// applyCounterpartyReturnDays sets the merchant's usual return window on a
// new purchase without one.
func applyCounterpartyReturnDays(tx *models.Transaction, cps []models.Counterparty) {
	if tx.ReturnUntil != nil || tx.Counterparty == nil || !models.TxWindowsEditable(tx) {
		return
	}
	for i := range cps {
		if cps[i].ID == tx.Counterparty.ID {
			tx.ReturnUntil = models.ReturnUntilFor(tx.ScheduledTo, &cps[i])
			return
		}
	}
}

// parseTxWindows reads the form's return / warranty days (whole purchase),
// nil when empty or unreadable.
func parseTxWindows(r *http.Request, purchase time.Time) (ret, war *time.Time) {
	ret, _ = models.ParseWindowValue(r.FormValue("return_until"), purchase)
	war, _ = models.ParseWindowValue(r.FormValue("warranty_until"), purchase)
	return ret, war
}

// formatDay is a window's day for a form, "" for none.
func formatDay(d *time.Time) string {
	if d == nil {
		return ""
	}
	return d.Format(time.DateOnly)
}
