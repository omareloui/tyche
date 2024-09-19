package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/omareloui/financial-planner/internal/db"
	"github.com/omareloui/financial-planner/internal/logger"
	"github.com/omareloui/financial-planner/internal/models"
	"go.uber.org/zap"
)

func (r *Repository) CreateTransaction(ctx context.Context, tx *models.Transaction, ftx *models.FrequentTx) error {
	l := logger.FromCtx(ctx)
	Txn, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer Txn.Commit()

	qtx := r.q.WithTx(Txn)

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
		l.Error("create transaction", zap.Error(err), zap.Any("transaction", tx))
		return err
	}
	l.Debug("created transaction", zap.Any("transaction", tx))

	if !tx.FulfilledOn.IsZero() {
		if err := r.ApplyTransactionToBalanceWithTxn(qtx, ctx, tx); err != nil {
			l.Error("applying transaction", zap.Error(err), zap.Any("transaction", tx))
			return err
		}
		l.Debug("applied transaction", zap.Any("transaction", tx))
	}

	if ftx != nil {
		if err := r.createFrequentTxWithTxn(qtx, ctx, ftx); err != nil {
			return err
		}
		if err := r.createFrequentTxTransactionWithTxn(qtx, ctx, ftx.ID, tx.ID); err != nil {
			return err
		}
	}

	return nil
}

func (r *Repository) GetTransactions(ctx context.Context) ([]models.Transaction, error) {
	l := logger.FromCtx(ctx)
	dbTransactions, err := r.q.GetTransactions(ctx)
	if err != nil {
		l.Error("querying transactions", zap.Error(err))
		return nil, err
	}
	l.Info("queried transactions")

	tx := make([]models.Transaction, len(dbTransactions))
	for i, t := range dbTransactions {
		tx[i] = *models.DBTransactionRowToTransaction(&t)
	}
	return tx, nil
}

func (r *Repository) GetTransaction(ctx context.Context, id string) (*models.Transaction, error) {
	l := logger.FromCtx(ctx)
	dbTx, err := r.q.GetTransaction(ctx, id)
	if err != nil {
		l.Error("querying transaction", zap.Error(err))
		return nil, err
	}
	l.Info("queried transaction")
	return models.DBTransactionRowToTransaction(&dbTx), nil
}

func (r *Repository) DeleteTransaction(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx)
	if err := r.q.DeleteTransaction(ctx, id); err != nil {
		l.Error("deleting transaction", zap.Error(err), zap.String("id", id))
		return err
	}
	l.Debug("deleted transaction", zap.String("id", id))
	return nil
}

func (r *Repository) UpdateTransaction(ctx context.Context, oldTx *models.Transaction, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)
	Txn, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer Txn.Commit()

	qtx := r.q.WithTx(Txn)

	if err := qtx.UpdateTransaction(ctx, db.UpdateTransactionParams{
		ID:          tx.ID,
		Amount:      tx.Amount,
		CurrencyID:  tx.Currency.ID,
		KindID:      tx.Kind.ID,
		ScheduledTo: tx.ScheduledTo,
	}); err != nil {
		l.Error("updating transaction", zap.Error(err), zap.Any("new_transaction", tx), zap.Any("old_transaction", oldTx))
		return err
	}
	l.Debug("updated transaction", zap.Any("new_transaction", tx), zap.Any("old_transaction", oldTx))

	if !oldTx.FulfilledOn.IsZero() {
		if err := r.UndoTransactionToBalanceWithTxn(qtx, ctx, oldTx); err != nil {
			return err
		}
		if err := r.ApplyTransactionToBalanceWithTxn(qtx, ctx, tx); err != nil {
			return err
		}
	}

	return nil
}

func (r *Repository) FulfillTransaction(ctx context.Context, id string, fulfilledOn time.Time) error {
	l := logger.FromCtx(ctx)
	if err := r.q.FulfillTransaction(ctx, db.FulfillTransactionParams{
		ID:          id,
		FulfilledOn: sql.NullTime{Time: fulfilledOn, Valid: !fulfilledOn.IsZero()},
	}); err != nil {
		l.Error("fulfilling transaction", zap.Error(err), zap.String("id", id), zap.Time("fulfilled_on", fulfilledOn))
		return err
	}
	l.Debug("fulfilled transaction", zap.String("id", id), zap.Time("fulfilled_on", fulfilledOn))
	return nil
}
