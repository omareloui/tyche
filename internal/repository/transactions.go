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

func (r *Repository) CreateTransaction(ctx context.Context, tx *models.Transaction, ftx *models.FrequentTx, scheduledTxs []models.Transaction) error {
	return r.transact(func(qtx *db.Queries) error {
		l := logger.FromCtx(ctx)

		if err := r.createTransactionWithTxn(ctx, qtx, tx); err != nil {
			return err
		}

		if !tx.FulfilledOn.IsZero() {
			if err := r.applyTxToAccountWithTxn(qtx, ctx, tx); err != nil {
				l.Error("applying transaction", zap.Error(err), zap.Any("transaction", tx))
				return err
			}
			l.Debug("applied transaction", zap.Any("transaction", tx))
		}

		if ftx != nil {
			if err := r.createFrequentTxWithTxn(qtx, ctx, ftx); err != nil {
				return err
			}
			if err := r.createFrequentTxTransactionWithTxn(ctx, qtx, ftx.ID, tx.ID); err != nil {
				return err
			}
		}

		for _, stx := range scheduledTxs {
			if err := r.createTransactionWithTxn(ctx, qtx, &stx); err != nil {
				return err
			}
			if err := r.createFrequentTxTransactionWithTxn(ctx, qtx, ftx.ID, stx.ID); err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *Repository) createTransactionWithTxn(ctx context.Context, qtx *db.Queries, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)
	if err := qtx.CreateTransaction(ctx, db.CreateTransactionParams{
		ID:          tx.ID,
		AccountID:   tx.Account.ID,
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
		tx[i] = *dbTransactionRowToTransaction(&t)
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
	return dbTransactionRowToTransaction(&dbTx), nil
}

func (r *Repository) DeleteTransaction(ctx context.Context, tx *models.Transaction) error {
	return r.transact(func(qtx *db.Queries) error {
		l := logger.FromCtx(ctx)
		if err := qtx.DeleteTransaction(ctx, tx.ID); err != nil {
			l.Error("deleting transaction", zap.Error(err), zap.String("id", tx.ID))
			return err
		}
		l.Debug("deleted transaction", zap.String("id", tx.ID))

		if !tx.FulfilledOn.IsZero() {
			if err := r.undoTxToAccountWithTxn(qtx, ctx, tx); err != nil {
				l.Error("undoing transaction after deletion", zap.Error(err))
				return err
			}
			l.Debug("undone transaction after deletion")
		}
		return nil
	})
}

func (r *Repository) UpdateTransaction(ctx context.Context, oldTx *models.Transaction, tx *models.Transaction) error {
	return r.transact(func(qtx *db.Queries) error {
		l := logger.FromCtx(ctx)

		if err := qtx.UpdateTransaction(ctx, db.UpdateTransactionParams{
			ID:          tx.ID,
			AccountID:   tx.Account.ID,
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
			if err := r.undoTxToAccountWithTxn(qtx, ctx, oldTx); err != nil {
				return err
			}
			if err := r.applyTxToAccountWithTxn(qtx, ctx, tx); err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *Repository) FulfillTransaction(ctx context.Context, tx *models.Transaction) error {
	return r.transact(func(qtx *db.Queries) error {
		l := logger.FromCtx(ctx)
		if err := qtx.FulfillTransaction(ctx, db.FulfillTransactionParams{
			ID:          tx.ID,
			FulfilledOn: sql.NullTime{Time: tx.FulfilledOn, Valid: !tx.FulfilledOn.IsZero()},
		}); err != nil {
			l.Error("fulfilling transaction", zap.Error(err), zap.String("id", tx.ID), zap.Time("fulfilled_on", tx.FulfilledOn))
			return err
		}
		l.Debug("fulfilled transaction", zap.String("id", tx.ID), zap.Time("fulfilled_on", tx.FulfilledOn))
		if err := r.applyTxToAccountWithTxn(qtx, ctx, tx); err != nil {
			l.Error("applying transaction to account", zap.Error(err))
			return err
		}
		l.Debug("applied transaction to account")
		return nil
	})
}

func dbTransactionRowToTransaction[T db.GetTransactionRow | db.GetTransactionsRow](dbTx *T) *models.Transaction {
	_dbTx := db.GetTransactionsRow(*dbTx)
	return &models.Transaction{
		ID: _dbTx.ID,
		Account: models.Account{
			ID:   _dbTx.AccountID,
			Name: _dbTx.AccountName.String,
			Currency: models.Currency{
				ID: _dbTx.AccountCurrencyID.String,
			},
		},
		Amount:      _dbTx.Amount,
		Description: _dbTx.Description.String,
		FulfilledOn: _dbTx.FulfilledOn.Time,
		ScheduledTo: _dbTx.ScheduledTo,
		CreatedAt:   _dbTx.CreatedAt.(time.Time),
		Currency: models.Currency{
			ID:        _dbTx.CurrencyID,
			Enum:      models.CurrencyEnum(_dbTx.Enum.Int64),
			EgpFxRate: _dbTx.EgpFxRate.Float64,
		},
		Kind: models.TransactionKind{
			ID:              _dbTx.KindID,
			Kind:            _dbTx.Kind.String,
			IncreaseBalance: _dbTx.IncreaseBalance.Bool,
		},
	}
}
