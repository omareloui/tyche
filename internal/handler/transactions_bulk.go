package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/oklog/ulid"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

// maxBulkTxs caps the transactions a bulk action (or the undo toast's
// deletes) takes at once.
const maxBulkTxs = 2000

// txBulkDoneEvent tells the selection (transactions.js) a bulk edit got
// saved, with how many were changed and skipped.
const txBulkDoneEvent = "tx-bulk-done"

var (
	errBulkNone     = resperr.New(http.StatusUnprocessableEntity, "No transactions selected.")
	errBulkTooMany  = resperr.New(http.StatusUnprocessableEntity, fmt.Sprintf("Too many transactions (over %d), narrow the filter.", maxBulkTxs))
	errBulkChanged  = resperr.New(http.StatusConflict, "The list changed meanwhile, check it again.")
	errBulkNoChange = resperr.New(http.StatusUnprocessableEntity, "Pick something to change.")
)

// bulkTxs is the request's selection: its ids (both legs of a transfer
// pair's row, transactions.js), or with all=1 every row the filter in the
// form matches (the page's query string), refused when it's not the expect
// rows the page offered. Ids already gone are skipped.
func (h Handler) bulkTxs(r *http.Request) ([]*models.Transaction, error) {
	if err := r.ParseForm(); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "Invalid form.")
	}
	var ids []string
	if r.PostForm.Get("all") == "1" {
		f := models.ParseTxFilter(r.PostForm)
		if f.IsZero() {
			return nil, errBulkNone
		}
		var rows int
		var err error
		if ids, rows, err = h.repo.GetFilteredTransactionIDs(r.Context(), f, maxBulkTxs+1); err != nil {
			return nil, err
		}
		expect, _ := strconv.Atoi(r.PostForm.Get("expect"))
		if rows > maxBulkTxs {
			return nil, errBulkTooMany
		} else if rows != expect {
			return nil, errBulkChanged
		}
	} else {
		ids = slices.Clone(r.PostForm["id"])
		// Up to both legs of each row.
		if len(ids) > 2*maxBulkTxs {
			return nil, errBulkTooMany
		}
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) == 0 {
		return nil, errBulkNone
	}

	txs := make([]*models.Transaction, 0, len(ids))
	for _, id := range ids {
		if _, err := ulid.ParseStrict(id); err != nil {
			return nil, errs.ErrInvalidID
		}
		tx, err := h.repo.GetTransaction(r.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			logger.FromCtx(r.Context()).Error("fetching transaction", zap.Error(err), zap.String("id", id))
			return nil, err
		}
		txs = append(txs, tx)
	}
	return txs, nil
}

// txSelDropEvent drops rows from the selection (transactions.js), with a
// notice telling why, if any.
const txSelDropEvent = "tx-sel-drop"

// BulkEditTransactionsForm is the bulk edit drawer of the selection
// (bulkTxs). The ones that can't be bulk edited (models.Transaction.BulkLock)
// are dropped from the selection, the drawer telling which; when none can,
// no drawer opens and the selection's notice tells it instead.
func (h Handler) BulkEditTransactionsForm(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	txs, err := h.bulkTxs(r)
	if err != nil {
		return nil, err
	}
	locks := map[models.TxBulkLock]int{}
	dropped := []string{}
	editable := 0
	for _, tx := range txs {
		if lock := tx.BulkLock(); lock != models.TxBulkLockNone {
			locks[lock]++
			dropped = append(dropped, tx.ID)
			continue
		}
		editable++
	}
	locked := models.DescribeBulkLocks(locks)

	if editable == 0 {
		w.Header().Set("HX-Reswap", "none")
		setHXTriggerJSON(w, map[string]any{txSelDropEvent: map[string]any{
			"ids": dropped, "message": "These can't be edited: " + locked + ".",
		}})
		return templ.NopComponent, nil
	}
	if len(dropped) > 0 {
		setHXTriggerJSON(w, map[string]any{txSelDropEvent: map[string]any{"ids": dropped}})
	}

	cats, cps, err := h.bulkEditDeps(r)
	if err != nil {
		return nil, err
	}
	return views.TxBulkEdit(&views.TxBulkEditFD{N: editable, Locked: locked}, cats, cps), nil
}

// setHXTriggerJSON sets the answer's HX-Trigger to events with details.
func setHXTriggerJSON(w http.ResponseWriter, events map[string]any) {
	b, err := json.Marshal(events)
	if err != nil {
		return
	}
	w.Header().Set("HX-Trigger", string(b))
}

func (h Handler) bulkEditDeps(r *http.Request) ([]models.Category, []models.Counterparty, error) {
	cats, err := h.repo.GetCategories(r.Context())
	if err != nil {
		return nil, nil, err
	}
	cps, err := h.repo.GetCounterparties(r.Context())
	if err != nil {
		return nil, nil, err
	}
	return cats, cps, nil
}

// BulkEditTransactions applies the form's changes to the selected
// transactions, skipping what doesn't fit one (models.TxBulkEdit.Apply),
// all at once. It answers the list, or the form with what went wrong.
func (h Handler) BulkEditTransactions(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())
	cats, cps, err := h.bulkEditDeps(r)
	if err != nil {
		return nil, err
	}
	if err := r.ParseForm(); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "Invalid form.")
	}
	fd := &views.TxBulkEditFD{
		Category:     r.PostForm.Get("category_id"),
		Counterparty: r.PostForm.Get("counterparty_id"),
		Date:         strings.TrimSpace(r.PostForm.Get("scheduled_to")),
		Reviewed:     r.PostForm.Get("reviewed") != "",
	}
	fd.N, _ = strconv.Atoi(r.PostForm.Get("n"))
	fd.Locked = r.PostForm.Get("locked")
	answer := func(msg string) (templ.Component, error) {
		fd.Error = msg
		return views.TxBulkEditForm(fd, cats, cps), nil
	}

	edit := models.TxBulkEdit{Reviewed: fd.Reviewed}
	switch fd.Category {
	case "":
	case models.TxFilterNone:
		edit.ClearCategory = true
	default:
		i := slices.IndexFunc(cats, func(c models.Category) bool { return c.ID == fd.Category })
		if i == -1 {
			return answer("That category is gone.")
		}
		edit.Category = &cats[i]
	}
	switch fd.Counterparty {
	case "":
	case models.TxFilterNone:
		edit.ClearCounterparty = true
	default:
		i := slices.IndexFunc(cps, func(c models.Counterparty) bool { return c.ID == fd.Counterparty })
		if i == -1 {
			return answer("That counterparty is gone.")
		}
		edit.Counterparty = &cps[i]
	}
	if fd.Date != "" {
		d, err := time.Parse(time.DateOnly, fd.Date)
		if err != nil {
			return answer("Invalid date.")
		}
		edit.Date = &d
	}
	if edit.IsZero() {
		return answer(errBulkNoChange.Message)
	}

	olds, err := h.bulkTxs(r)
	if re := (*resperr.RespError)(nil); errors.As(err, &re) {
		return answer(re.Message)
	} else if err != nil {
		return nil, err
	}

	today := h.userToday(r.Context())
	news := make([]*models.Transaction, 0, len(olds))
	var changed []*models.Transaction
	skipped := 0
	fulfilled := false
	for _, old := range olds {
		// Told on opening the form (BulkEditTransactionsForm), left out
		// silently, e.g. of all of the filter's.
		if old.BulkLock() != models.TxBulkLockNone {
			continue
		}
		tx, skips := edit.Apply(old, today)
		if len(skips) > 0 {
			skipped++
		}
		if tx == nil {
			continue
		}
		news = append(news, tx)
		changed = append(changed, old)
		fulfilled = fulfilled || !old.FulfilledAt.IsZero()
	}
	if err := h.repo.BulkUpdateTransactions(r.Context(), changed, news, edit.Reviewed); err != nil {
		l.Error("bulk updating transactions", zap.Error(err))
		return nil, err
	}
	l.Info("bulk updated transactions", zap.Int("count", len(news)), zap.Int("skipped", skipped))

	hxRetargetTxList(w)
	setHXTriggerJSON(w, map[string]any{
		drawerSavedEvent: map[string]any{},
		txBulkDoneEvent:  map[string]any{"updated": len(news), "skipped": skipped},
	})
	if fulfilled {
		if err := h.getAndRenderBalancesOOB(w, r); err != nil {
			return nil, err
		}
	}
	return h.transactionsList(r)
}
