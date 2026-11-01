package repository

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

func fxRate(from, to string) float64 {
	v, _ := fxrate.GetFxRate(from, to)
	return v
}

// pairTransferWithTxn pairs the SMS transaction tx, an expense or an income,
// with the other leg of its transfer between the user's accounts, if one was
// already received. Returns whether it got paired.
func (r *Repository) pairTransferWithTxn(ctx context.Context, qtx *queries.Queries, tx *models.Transaction) (bool, error) {
	l := logger.FromCtx(ctx)

	var other models.TxKind
	switch tx.Kind {
	case models.ExpenseTxKind:
		other = models.IncomeTxKind
	case models.IncomeTxKind:
		other = models.ExpenseTxKind
	default:
		return false, nil
	}
	if tx.FulfilledAt.IsZero() || tx.IsTransferLeg() {
		return false, nil
	}

	rows, err := qtx.GetTransferPairCandidates(ctx, queries.GetTransferPairCandidatesParams{
		Kind:      int64(other),
		AccountID: tx.Account.ID,
		ID:        tx.ID,
		Since:     tx.FulfilledAt.Add(-models.TransferPairWindow),
		Until:     tx.FulfilledAt.Add(models.TransferPairWindow),
		SmsOnly:   true,
	})
	if err != nil {
		l.Error("querying transfer pair candidates", zap.Error(err), zap.String("id", tx.ID))
		return false, err
	}
	candidates := make([]models.Transaction, len(rows))
	for i := range rows {
		candidates[i] = *dbTransactionRowToTransaction(&rows[i])
	}

	pair := models.PickTransferPair(tx, candidates, fxRate)
	if pair == nil {
		return false, nil
	}

	if err := qtx.SetTransferPair(ctx, queries.SetTransferPairParams{A: tx.ID, B: pair.ID}); err != nil {
		l.Error("pairing transfer", zap.Error(err), zap.String("id", tx.ID), zap.String("pair_id", pair.ID))
		return false, err
	}
	tx.TransferPairID = pair.ID
	l.Info("paired transfer", zap.String("id", tx.ID), zap.String("pair_id", pair.ID))
	return true, nil
}

// PairTransfers pairs the SMS transactions already received that are the two
// legs of a transfer between the user's accounts. Returns how many pairs.
func (r *Repository) PairTransfers(ctx context.Context) (int, error) {
	n := 0
	err := r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)

		ids, err := qtx.GetUnpairedSMSTransactions(ctx)
		if err != nil {
			l.Error("querying unpaired sms transactions", zap.Error(err))
			return err
		}

		paired := make(map[string]bool)
		for _, id := range ids {
			if paired[id] {
				continue
			}
			row, err := qtx.GetTransaction(ctx, id)
			if err != nil {
				l.Error("querying transaction to pair", zap.Error(err), zap.String("id", id))
				return err
			}
			tx := dbTransactionRowToTransaction(&row)
			ok, err := r.pairTransferWithTxn(ctx, qtx, tx)
			if err != nil {
				return err
			}
			if ok {
				paired[tx.ID], paired[tx.TransferPairID] = true, true
				n++
			}
		}
		return nil
	})
	return n, err
}

// manualPairWindow is how far apart, from the transaction, the other side of
// a transfer is offered when pairing by hand.
const manualPairWindow = 7 * 24 * time.Hour

// GetTransferPairCandidates returns what tx can be paired with by hand as
// the two sides of a transfer: the opposite kind on another account, within
// a week of it, closest in time first.
func (r *Repository) GetTransferPairCandidates(ctx context.Context, tx *models.Transaction) ([]models.Transaction, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	other := models.IncomeTxKind
	if tx.Kind == models.IncomeTxKind {
		other = models.ExpenseTxKind
	}
	at := tx.FulfilledAt
	if at.IsZero() {
		at = tx.ScheduledTo
	}

	rows, err := q.GetTransferPairCandidates(ctx, queries.GetTransferPairCandidatesParams{
		Kind:      int64(other),
		AccountID: tx.Account.ID,
		ID:        tx.ID,
		Since:     at.Add(-manualPairWindow),
		Until:     at.Add(manualPairWindow),
	})
	if err != nil {
		l.Error("querying transfer pair candidates", zap.Error(err), zap.String("id", tx.ID))
		return nil, err
	}

	txs := make([]models.Transaction, len(rows))
	for i := range rows {
		txs[i] = *dbTransactionRowToTransaction(&rows[i])
	}
	slices.SortStableFunc(txs, func(a, b models.Transaction) int {
		return cmp.Compare(a.FulfilledAt.Sub(at).Abs(), b.FulfilledAt.Sub(at).Abs())
	})
	return txs, nil
}

// LinkTransferPair pairs id and otherID, an expense and an income in any
// order, as the two sides of a transfer between the user's accounts.
func (r *Repository) LinkTransferPair(ctx context.Context, id, otherID string) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)

		var legs [2]*models.Transaction
		for i, txID := range []string{id, otherID} {
			row, err := qtx.GetTransaction(ctx, txID)
			if err != nil {
				l.Error("querying transaction to pair", zap.Error(err), zap.String("id", txID))
				return err
			}
			legs[i] = dbTransactionRowToTransaction(&row)
		}

		out, in := legs[0], legs[1]
		if out.Kind == models.IncomeTxKind {
			out, in = in, out
		}
		if err := models.ValidateTransferPair(out, in); err != nil {
			return err
		}

		if err := qtx.SetTransferPair(ctx, queries.SetTransferPairParams{A: out.ID, B: in.ID}); err != nil {
			l.Error("pairing transfer", zap.Error(err), zap.String("id", out.ID), zap.String("pair_id", in.ID))
			return err
		}
		l.Debug("paired transfer by hand", zap.String("id", out.ID), zap.String("pair_id", in.ID))
		return nil
	})
}

// UnlinkTransferPair makes both legs of the transfer pair of id plain
// transactions again.
func (r *Repository) UnlinkTransferPair(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	if err := q.ClearTransferPair(ctx, id); err != nil {
		l.Error("unlinking transfer pair", zap.Error(err), zap.String("id", id))
		return err
	}
	l.Debug("unlinked transfer pair", zap.String("id", id))
	return nil
}

// loadTransferPairs nests the incoming legs of the outgoing ones in lists
// under them. They're left out of the list query, so an outgoing leg gets
// its pair even when it's in another chunk.
func (r *Repository) loadTransferPairs(ctx context.Context, q *queries.Queries, lists ...[]models.Transaction) error {
	var ids []string
	for _, txs := range lists {
		for _, t := range txs {
			if t.IsTransferLeg() && t.Kind == models.ExpenseTxKind {
				ids = append(ids, t.ID)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}

	b, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	rows, err := q.GetTransferPairsOfMany(ctx, string(b))
	if err != nil {
		logger.FromCtx(ctx).Error("querying transfer pairs", zap.Error(err))
		return err
	}

	byOut := make(map[string]*models.Transaction, len(rows))
	for i := range rows {
		in := dbTransactionRowToTransaction(&rows[i])
		byOut[in.TransferPairID] = in
	}
	for _, txs := range lists {
		for i := range txs {
			if in, ok := byOut[txs[i].ID]; ok {
				txs[i].TransferPair = in
			}
		}
	}
	return nil
}

// syncTransferPairOnUpdateWithTxn unpairs a transaction (and its other leg)
// once an update leaves it unable to be a transfer leg: no longer an expense
// or income, a goal's, or moved to the other leg's account.
func (r *Repository) syncTransferPairOnUpdateWithTxn(qtx *queries.Queries, ctx context.Context, oldTx, tx *models.Transaction) error {
	if !oldTx.IsTransferLeg() {
		return nil
	}

	keep := tx.Kind == oldTx.Kind && tx.Goal == nil
	if keep {
		other, err := qtx.GetTransaction(ctx, oldTx.TransferPairID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			logger.FromCtx(ctx).Error("querying transfer pair", zap.Error(err), zap.String("id", oldTx.TransferPairID))
			return err
		}
		keep = other.Transaction.AccountID != tx.Account.ID
	}
	if keep {
		return nil
	}

	if err := qtx.ClearTransferPair(ctx, tx.ID); err != nil {
		logger.FromCtx(ctx).Error("unlinking transfer pair", zap.Error(err), zap.String("id", tx.ID))
		return err
	}
	logger.FromCtx(ctx).Debug("unlinked transfer pair after update", zap.String("id", tx.ID))
	return nil
}
