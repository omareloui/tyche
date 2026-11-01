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

var linkTransferPairErrs = []error{
	models.ErrTransferPairKinds,
	models.ErrTransferPairAccount,
	models.ErrTransferPairNotDone,
	models.ErrTransferPairLinked,
	models.ErrTransferPairAlready,
}

func (h Handler) GetTransferPairPicker(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return h.renderTransferPairPicker(r, "")
}

// LinkTransferPair pairs the transaction with other_id by hand, as the two
// sides of a transfer between the user's accounts.
func (h Handler) LinkTransferPair(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	otherID := r.FormValue("other_id")
	if id == "" || otherID == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	if err := h.repo.LinkTransferPair(r.Context(), id, otherID); err != nil {
		for _, e := range linkTransferPairErrs {
			if errors.Is(err, e) {
				return h.renderTransferPairPicker(r, e.Error())
			}
		}
		l.Error("pairing transfer", zap.Error(err))
		return nil, err
	}
	l.Info("paired transfer by hand")

	// The income moves under the expense.
	if err := h.renderTransactionsListOOB(w, r); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h Handler) renderTransferPairPicker(r *http.Request, errMsg string) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		logger.FromCtx(r.Context()).Error("fetching transaction", zap.Error(err))
		return nil, err
	}

	cands, err := h.repo.GetTransferPairCandidates(r.Context(), tx)
	if err != nil {
		return nil, err
	}
	return views.TransferPairPicker(tx, cands, errMsg), nil
}

// UnlinkTransferPair makes both legs of a transfer pair a plain expense and
// income again.
func (h Handler) UnlinkTransferPair(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	if err := h.repo.UnlinkTransferPair(r.Context(), id); err != nil {
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("unlinked transfer pair")

	// The income goes back to its own row.
	if err := h.renderTransactionsListOOB(w, r); err != nil {
		return nil, err
	}
	return nil, nil
}

// PairTransfers pairs the SMS transactions already received that are the two
// legs of a transfer between the user's accounts.
func (h Handler) PairTransfers(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	n, err := h.repo.PairTransfers(r.Context())
	if err != nil {
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("paired transfers", zap.Int("count", n))
	return views.TransferPairsCard(&n), nil
}
