package handler

import (
	"database/sql"
	"errors"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

const (
	// wishTxCandidates is how many expenses "Already bought" offers.
	wishTxCandidates = 30
	// wishLikelyDays is how recent an expense covering a wish's price is
	// flagged likely.
	wishLikelyDays = 60
)

// GetWishBought opens the picker of the transaction a waiting wish was
// already bought in; its search answers the list alone.
func (h Handler) GetWishBought(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	item, err := h.waitingWish(r)
	if err != nil {
		return nil, err
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	txs, err := h.repo.GetWishTxCandidates(r.Context(), q, wishTxCandidates)
	if err != nil {
		return nil, err
	}
	cands := flagWishTxCandidates(item, txs)
	if r.Header.Get("HX-Target") == views.WishBoughtListID {
		return views.WishTxCandidates(item, cands), nil
	}
	return views.WishBoughtDrawer(item, cands, q), nil
}

// flagWishTxCandidates flags the expenses covering the wish's price (in their
// account's currency) and recent, putting them first, then newest first.
func flagWishTxCandidates(item *models.WishlistItem, txs []models.Transaction) []views.WishTxCandidate {
	since := time.Now().AddDate(0, 0, -wishLikelyDays)
	cands := make([]views.WishTxCandidate, len(txs))
	for i, t := range txs {
		price := item.Price * wishlistFxRate(item, &t.Account)
		cands[i] = views.WishTxCandidate{
			Tx:     t,
			Likely: math.Abs(t.Amount)+0.01 >= price*0.95 && t.ScheduledTo.After(since),
		}
	}
	// The likely ones first, each group newest first.
	slices.SortStableFunc(cands, func(a, b views.WishTxCandidate) int {
		if a.Likely != b.Likely {
			if a.Likely {
				return -1
			}
			return 1
		}
		return b.Tx.ScheduledTo.Compare(a.Tx.ScheduledTo)
	})
	return cands
}

// SetWishBought marks the waiting wish bought in the picked transaction.
func (h Handler) SetWishBought(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	item, err := h.waitingWish(r)
	if err != nil {
		return nil, err
	}
	tx, err := h.wishTx(r, r.FormValue("tx_id"))
	if err != nil {
		return nil, err
	}
	if err := h.linkWishes(r, tx.ID, item.ID); err != nil {
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("wish bought in a transaction", zap.String("wish", item.ID), zap.String("transaction_id", tx.ID))
	drawerSaved(w, "#wishlist", "outerHTML")
	return h.wishlist(r, false)
}

// wishTx is a transaction wishes can be bought in, a 422 when it can't hold
// them.
func (h Handler) wishTx(r *http.Request, id string) (*models.Transaction, error) {
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	tx, err := h.repo.GetTransaction(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, resperr.New(http.StatusNotFound, "Transaction not found")
	}
	if err != nil {
		return nil, err
	}
	if !tx.CanHoldWishes() {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Only an expense of yours can hold wishlist items.")
	}
	return tx, nil
}

func (h Handler) linkWishes(r *http.Request, txID string, wishIDs ...string) error {
	err := h.repo.LinkWishesToTx(r.Context(), txID, wishIDs)
	if errors.Is(err, repository.ErrWishNotWaiting) {
		return resperr.New(http.StatusConflict, "This wish is already decided.")
	}
	return err
}

// GetTxWishes lists the waiting wishes to mark bought in the transaction,
// from its details.
func (h Handler) GetTxWishes(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	tx, err := h.wishTx(r, r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	items, err := h.repo.GetWishlistItems(r.Context())
	if err != nil {
		return nil, err
	}
	waiting := slices.DeleteFunc(items, func(it models.WishlistItem) bool { return it.Status != models.WishlistWaiting })
	return views.TxWishPicker(tx, waiting), nil
}

// AddTxWish marks a waiting wish bought in the transaction, answering the
// details' items.
func (h Handler) AddTxWish(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	tx, err := h.wishTx(r, r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	wishID := r.FormValue("wish_id")
	if wishID == "" {
		return nil, errs.ErrInvalidID
	}
	if err := h.linkWishes(r, tx.ID, wishID); err != nil {
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("wish bought in a transaction", zap.String("wish", wishID), zap.String("transaction_id", tx.ID))
	return h.txItemsAnswer(w, r, tx.ID, nil, nil)
}

// DeleteTxWish puts a wish bought in the transaction back on the wishlist,
// answering the details' items.
func (h Handler) DeleteTxWish(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	tx, err := h.wishTx(r, r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	err = h.repo.UnlinkWishFromTx(r.Context(), tx.ID, r.PathValue("wishID"))
	if errors.Is(err, repository.ErrWishTxNotFound) {
		return nil, resperr.New(http.StatusNotFound, "Wish not found")
	}
	if err != nil {
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("wish unlinked from a transaction", zap.String("wish", r.PathValue("wishID")), zap.String("transaction_id", tx.ID))
	return h.txItemsAnswer(w, r, tx.ID, nil, nil)
}
