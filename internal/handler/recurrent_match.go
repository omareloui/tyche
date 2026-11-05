package handler

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

// GetRecurrentMatchPicker lists the scheduled transactions of recurrent ones
// an SMS transaction may be the payment of, to match it by hand.
func (h Handler) GetRecurrentMatchPicker(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	tx, err := h.recurrentMatchTx(r)
	if err != nil {
		return nil, err
	}
	cands, err := h.repo.GetRecurrentMatchCandidates(r.Context(), tx)
	if err != nil {
		return nil, err
	}
	return views.RecurrentMatchPicker(tx, cands), nil
}

// MatchRecurrentOccurrence makes the scheduled transaction occurrence_id the
// one the SMS transaction paid: it takes its amount, date and SMS, the SMS
// transaction is deleted.
func (h Handler) MatchRecurrentOccurrence(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	tx, err := h.recurrentMatchTx(r)
	if err != nil {
		return nil, err
	}
	occ, err := h.repo.GetTransaction(r.Context(), r.FormValue("occurrence_id"))
	if err != nil {
		return nil, err
	}
	if occ.RecurrentTxID == "" || occ.IsTransferLeg() || !models.MayPayOccurrence(tx, occ) ||
		(!occ.FulfilledAt.IsZero() && occ.FromSMS) {
		return nil, resperr.New(http.StatusUnprocessableEntity, "It can't be the payment of that one.")
	}

	if err := h.repo.MatchSMSToOccurrence(r.Context(), tx, occ); err != nil {
		logger.FromCtx(r.Context()).Error("matching an sms to an occurrence", zap.Error(err))
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("matched an sms to an occurrence by hand", zap.String("id", occ.ID), zap.String("sms_tx_id", tx.ID))

	if err := h.renderTransactionsListOOB(w, r); err != nil {
		return nil, err
	}
	if err := h.getAndRenderBalancesOOB(w, r); err != nil {
		return nil, err
	}
	return nil, nil
}

// recurrentMatchTx is the path's transaction, if it can be matched with a
// recurrent one's scheduled transaction (views.CanMatchRecurrent).
func (h Handler) recurrentMatchTx(r *http.Request) (*models.Transaction, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		logger.FromCtx(r.Context()).Error("fetching transaction", zap.Error(err))
		return nil, err
	}
	if !views.CanMatchRecurrent(tx) {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Only an SMS's expense or income can be matched.")
	}
	return tx, nil
}
