package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// nullDay is a day column's value, NULL for nil.
func nullDay(d *time.Time) sql.NullTime {
	if d == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *d, Valid: true}
}

// dayPtr reads a day column, nil for NULL.
func dayPtr(d sql.NullTime) *time.Time {
	if !d.Valid {
		return nil
	}
	t := d.Time.UTC()
	return &t
}

// parseDay reads a "YYYY-MM-DD…" text day, nil when empty or unreadable.
func parseDay(s string) *time.Time {
	if len(s) < 10 {
		return nil
	}
	d, err := time.Parse(time.DateOnly, s[:10])
	if err != nil {
		return nil
	}
	return &d
}

// itemWarrantyParam is an item line's own warranty: never a charge's, nor a
// recurrent transaction's (each occurrence is its own purchase).
func itemWarrantyParam(it *models.TxItem, owner itemOwner) sql.NullTime {
	if it.Kind.IsCharge() || owner.txID == "" {
		return sql.NullTime{}
	}
	return nullDay(it.WarrantyUntil)
}

// SetTransactionWindows sets an expense's return / warranty windows and
// returned day (its details' section).
func (r *Repository) SetTransactionWindows(ctx context.Context, id string, ret, war, returned *time.Time) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	if err := q.SetTransactionWindows(ctx, queries.SetTransactionWindowsParams{
		ID:            id,
		ReturnUntil:   nullDay(ret),
		WarrantyUntil: nullDay(war),
		ReturnedAt:    nullDay(returned),
	}); err != nil {
		logger.FromCtx(ctx).Error("setting transaction windows", zap.Error(err), zap.String("id", id))
		return err
	}
	return nil
}

// SetTxItemWarranty sets an item line's own warranty, ErrItemNotFound when
// it isn't an item line of the transaction.
func (r *Repository) SetTxItemWarranty(ctx context.Context, txID, itemID string, d *time.Time) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	n, err := q.SetTransactionItemWarranty(ctx, queries.SetTransactionItemWarrantyParams{
		ID:            itemID,
		TransactionID: sql.NullString{String: txID, Valid: true},
		WarrantyUntil: nullDay(d),
	})
	if err != nil {
		logger.FromCtx(ctx).Error("setting item warranty", zap.Error(err), zap.String("id", itemID))
		return err
	}
	if n == 0 {
		return ErrItemNotFound
	}
	return nil
}

// ReturnRefundCandidate is a refund that looks like the expense's return:
// at its counterparty, into its account, from its day through its return
// window, of at least half its amount; "" when none.
func (r *Repository) ReturnRefundCandidate(ctx context.Context, tx *models.Transaction) (string, error) {
	if tx.ReturnUntil == nil || tx.ReturnedAt != nil || tx.Counterparty == nil || tx.Counterparty.ID == "" {
		return "", nil
	}
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return "", err
	}
	from := time.Date(tx.ScheduledTo.Year(), tx.ScheduledTo.Month(), tx.ScheduledTo.Day(), 0, 0, 0, 0, time.UTC)
	id, err := q.GetReturnRefundCandidate(ctx, queries.GetReturnRefundCandidateParams{
		CounterpartyID: sql.NullString{String: tx.Counterparty.ID, Valid: true},
		AccountID:      tx.Account.ID,
		FromDay:        from,
		Until:          tx.ReturnUntil.AddDate(0, 0, 1),
		MinAmount:      tx.Amount / 2,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		logger.FromCtx(ctx).Error("getting return refund", zap.Error(err), zap.String("id", tx.ID))
		return "", err
	}
	return id, nil
}
