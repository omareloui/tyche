package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

// maxSplitPeople is how many people a bill can be split with.
const maxSplitPeople = 30

// splittableTx is the split of the path's transaction, a 422 when it can't
// be split.
func (h Handler) splittableTx(r *http.Request) (*models.TxSplit, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	s, err := h.repo.GetTxSplit(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, resperr.New(http.StatusNotFound, "Transaction not found")
	}
	if err != nil {
		logger.FromCtx(r.Context()).Error("fetching split", zap.Error(err), zap.String("id", id))
		return nil, err
	}
	if !s.Tx.CanSplit() {
		return nil, resperr.New(http.StatusUnprocessableEntity, models.ErrSplitNotSplittable.Error())
	}
	return s, nil
}

// splitAnswer is the split panel, in the open details drawer, or a drawer of
// its own.
func (h Handler) splitAnswer(r *http.Request, s *models.TxSplit, msg views.TxSplitMsg) (templ.Component, error) {
	cps, err := h.repo.GetCounterparties(r.Context())
	if err != nil {
		logger.FromCtx(r.Context()).Error("fetching counterparties", zap.Error(err))
		return nil, err
	}
	if r.Header.Get("HX-Target") == txDetailsPanel {
		return views.TxSplitPanel(s, cps, msg), nil
	}
	return views.TxSplitDrawer(s, cps, msg), nil
}

// GetTxSplit opens the split of a transaction's bill.
func (h Handler) GetTxSplit(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	s, err := h.splittableTx(r)
	if err != nil {
		return nil, err
	}
	return h.splitAnswer(r, s, views.TxSplitMsg{})
}

// SaveTxSplit saves the split as changed: the people (person, a new one
// add_person, one to remove) and the items' shares (share_<item id>: ids, or
// id:units for an item shared by its units).
func (h Handler) SaveTxSplit(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	s, err := h.splittableTx(r)
	if err != nil {
		return nil, err
	}
	l := logger.FromCtx(r.Context())
	if err := r.ParseForm(); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "Invalid form")
	}

	known := map[string]bool{}
	for _, p := range s.People {
		known[p.ID] = true
	}
	cps, err := h.repo.GetCounterparties(r.Context())
	if err != nil {
		l.Error("fetching counterparties", zap.Error(err))
		return nil, err
	}
	for _, cp := range cps {
		known[cp.ID] = true
	}

	remove := r.FormValue("remove")
	var people []string
	for _, id := range append(r.Form["person"], strings.TrimSpace(r.FormValue("add_person"))) {
		if id == "" || id == remove || !known[id] || slices.Contains(people, id) {
			continue
		}
		people = append(people, id)
	}
	if len(people) > maxSplitPeople {
		return h.splitAnswer(r, s, views.TxSplitMsg{Error: "That's too many people for one bill."})
	}

	shares := map[string]models.SplitItemShare{}
	for _, it := range s.Tx.Items {
		if it.Kind.IsCharge() {
			continue
		}
		var qty float64
		if models.ItemByUnits(&it) {
			qty = it.Quantity
		}
		shares[it.ID] = models.ParseSplitItemShare(r.Form["share_"+it.ID], people, qty)
	}

	if err := h.repo.SaveTxSplit(r.Context(), s.Tx.ID, people, shares); err != nil {
		l.Error("saving split", zap.Error(err))
		return nil, err
	}
	l.Info("saved split", zap.Int("people", len(people)))

	s, err = h.repo.GetTxSplit(r.Context(), s.Tx.ID)
	if err != nil {
		return nil, err
	}
	return h.splitAnswer(r, s, views.TxSplitMsg{})
}

// RequestTxSplitPayments schedules the incomes reimbursing the expense of
// what's left to request of each share, answering the split again (and the
// list, where they're nested under the expense, out of band).
func (h Handler) RequestTxSplitPayments(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	s, err := h.splittableTx(r)
	if err != nil {
		return nil, err
	}
	l := logger.FromCtx(r.Context())

	res := s.Compute()
	n, err := h.repo.RequestSplitPayments(r.Context(), s.Tx, res.ToRequest(), h.userToday(r.Context()))
	if errors.Is(err, models.ErrReimbursementExceeds) {
		return h.splitAnswer(r, s, views.TxSplitMsg{Error: "The shares add up to more than the expense's amount: check its items."})
	}
	if err != nil {
		l.Error("requesting split payments", zap.Error(err))
		return nil, err
	}

	if err := h.renderTransactionsListOOB(w, r); err != nil {
		return nil, err
	}
	s, err = h.repo.GetTxSplit(r.Context(), s.Tx.ID)
	if err != nil {
		return nil, err
	}
	return h.splitAnswer(r, s, views.TxSplitMsg{Notice: views.SplitRequestedNotice(n)})
}
