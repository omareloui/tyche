package handler

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

var linkReimbursementErrs = []error{
	models.ErrReimbursementKinds,
	models.ErrReimbursementGoal,
	models.ErrReimbursementCurrency,
	models.ErrReimbursementExceeds,
	models.ErrReimbursementTransferPair,
}

func (h Handler) GetReimbursePicker(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return h.renderReimbursePicker(r, "")
}

func (h Handler) LinkReimbursement(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	expenseID := r.FormValue("expense_id")
	if id == "" || expenseID == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	if err := h.repo.LinkReimbursement(r.Context(), id, expenseID); err != nil {
		for _, e := range linkReimbursementErrs {
			if errors.Is(err, e) {
				return h.renderReimbursePicker(r, e.Error())
			}
		}
		l.Error("linking reimbursement", zap.Error(err))
		return nil, err
	}
	l.Info("linked reimbursement")

	// The income moves under its expense.
	if err := h.renderTransactionsListOOB(w, r); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h Handler) UnlinkReimbursement(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	if err := h.repo.UnlinkReimbursement(r.Context(), id); err != nil {
		return nil, err
	}
	l.Info("unlinked reimbursement")

	// The income goes back to its own day.
	if err := h.renderTransactionsListOOB(w, r); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h Handler) renderReimbursePicker(r *http.Request, errMsg string) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		l.Error("fetching transaction", zap.Error(err))
		return nil, err
	}

	exps, err := h.repo.GetReimbursableExpenses(r.Context(), tx)
	if err != nil {
		return nil, err
	}
	return views.ReimbursePicker(tx, exps, errMsg), nil
}
