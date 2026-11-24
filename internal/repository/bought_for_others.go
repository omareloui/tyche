package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// CreateForOthers saves an expense the user bought for someone else
// (tx.ForPerson, nil when unnamed), out of every analysis; payback schedules
// an income reimbursing all of it (see setForOthersWithTxn for its day).
func (r *Repository) CreateForOthers(ctx context.Context, tx *models.Transaction, payback bool, today time.Time) error {
	if !tx.CanBeForOthers() {
		return models.ErrForOthersKind
	}
	return r.transact(ctx, func(qtx *queries.Queries) error {
		if err := r.createTransactionAndApplyToAccountIfNeededWithTxn(ctx, qtx, tx); err != nil {
			return err
		}
		if err := autoTagWithTxn(ctx, qtx, tx); err != nil {
			return err
		}
		return r.setForOthersWithTxn(ctx, qtx, tx, true, tx.ForPerson, payback, today)
	})
}

// SetForOthers marks the expense id bought for person (nil when unnamed) or,
// on false, for the user again (the incomes already reimbursing it stay,
// netting it as usual). payback schedules an income for what isn't
// reimbursed yet, so asking twice schedules nothing more.
func (r *Repository) SetForOthers(ctx context.Context, id string, on bool, person *models.Counterparty, payback bool, today time.Time) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		tx, err := r.getTransactionWithTxn(ctx, qtx, id)
		if err != nil {
			return err
		}
		if on {
			boughtBy, err := qtx.GetBoughtForTotal(ctx, queries.GetBoughtForTotalParams{
				ID: sql.NullString{String: id, Valid: true},
			})
			if err != nil {
				logger.FromCtx(ctx).Error("querying bought for total", zap.Error(err), zap.String("id", id))
				return err
			}
			if boughtBy > 0 {
				return models.ErrForOthersBoughtBy
			}
			people, err := qtx.CountTxSplitPeople(ctx, id)
			if err != nil {
				logger.FromCtx(ctx).Error("counting split people", zap.Error(err), zap.String("id", id))
				return err
			}
			tx.SplitPeople = int(people)
			if err := models.ValidateForOthers(tx); err != nil {
				return err
			}
		}
		return r.setForOthersWithTxn(ctx, qtx, tx, on, person, on && payback, today)
	})
}

// setForOthersWithTxn sets the flag; payback schedules the income paying
// back what isn't reimbursed yet, today or on the expense's day when ahead.
func (r *Repository) setForOthersWithTxn(ctx context.Context, qtx *queries.Queries, tx *models.Transaction, on bool, person *models.Counterparty, payback bool, today time.Time) error {
	l := logger.FromCtx(ctx)

	var personID string
	if on && person != nil {
		personID = person.ID
	}
	if err := qtx.SetTransactionForOthers(ctx, queries.SetTransactionForOthersParams{
		ID:          tx.ID,
		ForOthers:   on,
		ForPersonID: sql.NullString{String: personID, Valid: personID != ""},
	}); err != nil {
		l.Error("setting bought for others", zap.Error(err), zap.String("id", tx.ID))
		return err
	}
	tx.ForOthers = on
	tx.ForPerson = nil
	if personID != "" {
		tx.ForPerson = person
	}
	if !payback {
		return nil
	}

	reimbursed, err := qtx.GetReimbursedTotal(ctx, queries.GetReimbursedTotalParams{
		ID: sql.NullString{String: tx.ID, Valid: true},
	})
	if err != nil {
		l.Error("querying reimbursed total", zap.Error(err), zap.String("id", tx.ID))
		return err
	}
	amount := tx.Account.Currency.Round(tx.Amount - reimbursed)
	if amount <= amountEpsilon {
		return nil
	}
	day := today
	if tx.ScheduledTo.After(today) {
		day = tx.ScheduledTo
	}
	if err := r.scheduleReimbursementWithTxn(ctx, qtx, tx, personID, amount, day, models.ForOthersDescription(tx)); err != nil {
		return err
	}
	tx.Reimbursed = reimbursed + amount
	l.Info("scheduled the payback of a purchase for others", zap.String("transaction_id", tx.ID))
	return nil
}

// loadForPersons names who the transactions were bought for.
func loadForPersons(ctx context.Context, q *queries.Queries, lists ...[]models.Transaction) error {
	seen := map[string]bool{}
	var ids []string
	for _, txs := range lists {
		for i := range txs {
			if p := txs[i].ForPerson; p != nil && !seen[p.ID] {
				seen[p.ID] = true
				ids = append(ids, p.ID)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.GetCounterpartyNamesOf(ctx, ids)
	if err != nil {
		logger.FromCtx(ctx).Error("querying who transactions were bought for", zap.Error(err))
		return err
	}
	names := make(map[string]string, len(rows))
	for _, row := range rows {
		names[row.ID] = row.Name
	}
	for _, txs := range lists {
		for i := range txs {
			if p := txs[i].ForPerson; p != nil {
				p.Name = names[p.ID]
			}
		}
	}
	return nil
}
