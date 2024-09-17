package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/omareloui/financial-planner/internal/db"
	"github.com/omareloui/financial-planner/internal/models"
	"go.uber.org/zap"
)

func (r *Repository) CreateTransaction(ctx context.Context, tx *models.Transaction, ftx *models.FrequentTransaction) error {
	txn, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer txn.Commit()

	qtx := r.q.WithTx(txn)

	if err := qtx.CreateTransaction(ctx, db.CreateTransactionParams{
		ID:          tx.ID,
		Amount:      tx.Amount,
		KindID:      tx.Kind.ID,
		CurrencyID:  tx.Currency.ID,
		Description: sql.NullString{String: tx.Description, Valid: tx.Description != ""},
		ScheduledTo: tx.ScheduledTo,
		FulfilledOn: sql.NullTime{Time: tx.FulfilledOn, Valid: !tx.FulfilledOn.IsZero()},
		CreatedAt:   tx.CreatedAt,
	}); err != nil {
		r.l.Error("create transaction", zap.Error(err), zap.Any("transaction", tx))
		return err
	}
	r.l.Debug("created transaction", zap.Any("transaction", tx))

	if !tx.FulfilledOn.IsZero() {
		if err := r.ApplyTransactionToBalanceWithTXN(qtx, ctx, tx); err != nil {
			r.l.Error("applying transaction", zap.Error(err), zap.Any("transaction", tx))
			return err
		}
		r.l.Debug("applied transaction", zap.Any("transaction", tx))
	}

	if ftx != nil {
		if err := qtx.CreateFrequentTransaction(ctx, db.CreateFrequentTransactionParams{
			ID:                       ftx.ID,
			FrequencyBy:              int64(ftx.FrequencyBy),
			Frequency:                int64(ftx.Frequency),
			Amount:                   ftx.Amount,
			CurrencyID:               ftx.Currency.ID,
			KindID:                   ftx.Kind.ID,
			Description:              sql.NullString{String: ftx.Description, Valid: ftx.Description != ""},
			StopsAt:                  sql.NullTime{Time: ftx.StopsOn, Valid: !ftx.StopsOn.IsZero()},
			CreatedAt:                ftx.CreatedAt,
			LastTransactionCreatedAt: sql.NullTime{Time: ftx.LastTransactionCreatedAt, Valid: !ftx.LastTransactionCreatedAt.IsZero()},
		}); err != nil {
			r.l.Error("creating frequent transaction", zap.Error(err), zap.Any("frequent_transaction", ftx))
			return err
		}
		r.l.Debug("created frequent transaction", zap.Any("frequent_transaction", ftx))

		if err := qtx.CreateFrequentTransactionTransaction(ctx, db.CreateFrequentTransactionTransactionParams{
			TransactionID: tx.ID,
			FrequencyID:   ftx.ID,
		}); err != nil {
			r.l.Error("creating frequent transaction transaction", zap.Error(err), zap.Any("frequent_transaction", ftx), zap.Any("transaction", tx))
			return err
		}
		r.l.Debug("created frequent transaction transaction", zap.Any("frequent_transaction", ftx), zap.Any("transaction", tx))
	}

	return nil
}

func (r *Repository) GetTransactions(ctx context.Context) ([]models.Transaction, error) {
	dbTransactions, err := r.q.GetTransactions(ctx)
	if err != nil {
		return nil, err
	}

	tx := make([]models.Transaction, len(dbTransactions))
	for i, t := range dbTransactions {
		tx[i] = *models.DBTransactionsRowToTransaction(&t)
	}
	return tx, nil
}

func (r *Repository) GetTransaction(ctx context.Context, id string) (*models.Transaction, error) {
	dbTx, err := r.q.GetTransaction(ctx, id)
	if err != nil {
		return nil, err
	}
	return models.DBTransactionRowToTransaction(&dbTx), nil
}

func (r *Repository) DeleteTransaction(ctx context.Context, id string) error {
	if err := r.q.DeleteTransaction(ctx, id); err != nil {
		r.l.Error("deleting transaction", zap.Error(err), zap.String("id", id))
		return err
	}
	r.l.Debug("deleted transaction", zap.String("id", id))
	return nil
}

func (r *Repository) UpdateTransaction(ctx context.Context, oldTx *models.Transaction, tx *models.Transaction) error {
	txn, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer txn.Commit()

	qtx := r.q.WithTx(txn)

	if err := qtx.UpdateTransaction(ctx, db.UpdateTransactionParams{
		ID:          tx.ID,
		Amount:      tx.Amount,
		CurrencyID:  tx.Currency.ID,
		KindID:      tx.Kind.ID,
		ScheduledTo: tx.ScheduledTo,
	}); err != nil {
		r.l.Error("updating transaction", zap.Error(err), zap.Any("new_transaction", tx), zap.Any("old_transaction", oldTx))
		return err
	}
	r.l.Debug("updated transaction", zap.Any("new_transaction", tx), zap.Any("old_transaction", oldTx))

	if !oldTx.FulfilledOn.IsZero() {
		if err := r.UndoTransactionToBalanceWithTXN(qtx, ctx, oldTx); err != nil {
			return err
		}
		if err := r.ApplyTransactionToBalanceWithTXN(qtx, ctx, tx); err != nil {
			return err
		}
	}

	return nil
}

func (r *Repository) FulfillTransaction(ctx context.Context, id string, fulfilledOn time.Time) error {
	if err := r.q.FulfillTransaction(ctx, db.FulfillTransactionParams{
		ID:          id,
		FulfilledOn: sql.NullTime{Time: fulfilledOn, Valid: !fulfilledOn.IsZero()},
	}); err != nil {
		r.l.Error("fulfilling transaction", zap.Error(err), zap.String("id", id), zap.Time("fulfilled_on", fulfilledOn))
		return err
	}
	r.l.Debug("fulfilled transaction", zap.String("id", id), zap.Time("fulfilled_on", fulfilledOn))
	return nil
}
