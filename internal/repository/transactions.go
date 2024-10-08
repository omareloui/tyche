package repository

import (
	"context"
	"database/sql"

	"github.com/omareloui/tyche/internal/db"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"go.uber.org/zap"
)

func (r *Repository) CreateTransaction(ctx context.Context, tx *models.Transaction, ftx *models.FrequentTx, scheduledTxs []models.Transaction) error {
	return r.transact(func(qtx *db.Queries) error {
		if err := r.createTransactionAndApplyToAccountIfNeededWithTxn(ctx, qtx, tx); err != nil {
			return err
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

	catId := sql.NullString{}
	cpId := sql.NullString{}
	toAccId := sql.NullString{}

	if tx.Category != nil {
		catId.Valid = tx.Category.ID != ""
		catId.String = tx.Category.ID
	}
	if tx.Counterparty != nil {
		cpId.Valid = tx.Counterparty.ID != ""
		cpId.String = tx.Counterparty.ID
	}
	if tx.ToAccount != nil {
		toAccId.Valid = tx.ToAccount.ID != ""
		toAccId.String = tx.ToAccount.ID
	}

	if err := qtx.CreateTransaction(ctx, db.CreateTransactionParams{
		ID:             tx.ID,
		AccountID:      tx.Account.ID,
		Amount:         tx.Amount,
		KindID:         tx.Kind.ID,
		CategoryID:     catId,
		CounterpartyID: cpId,
		Description:    sql.NullString{String: tx.Description, Valid: tx.Description != ""},
		ToAccountID:    toAccId,
		ToAmount:       sql.NullFloat64{Float64: tx.ToAmount, Valid: tx.ToAmount != 0},
		ScheduledTo:    tx.ScheduledTo,
		FulfilledAt:    sql.NullTime{Time: tx.FulfilledAt, Valid: !tx.FulfilledAt.IsZero()},
	}); err != nil {
		l.Error("create transaction", zap.Error(err), zap.Any("transaction", tx))
		return err
	}
	l.Debug("created transaction", zap.Any("transaction", tx))
	return nil
}

func (r *Repository) createTransactionAndApplyToAccountIfNeededWithTxn(ctx context.Context, qtx *db.Queries, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)
	if err := r.createTransactionWithTxn(ctx, qtx, tx); err != nil {
		return err
	}

	if !tx.FulfilledAt.IsZero() {
		if err := r.applyTxToAccountWithTxn(qtx, ctx, tx); err != nil {
			l.Error("applying transaction", zap.Error(err), zap.Any("transaction", tx))
			return err
		}
		l.Debug("applied transaction", zap.Any("transaction", tx))
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

		if !tx.FulfilledAt.IsZero() {
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

		catId := sql.NullString{}
		cpId := sql.NullString{}
		toAccId := sql.NullString{}

		if tx.Category != nil {
			catId.Valid = tx.Category.ID != ""
			catId.String = tx.Category.ID
		}

		if tx.Counterparty != nil {
			cpId.Valid = tx.Counterparty.ID != ""
			cpId.String = tx.Counterparty.ID
		}

		if tx.ToAccount != nil {
			toAccId.Valid = tx.ToAccount.ID != ""
			toAccId.String = tx.ToAccount.ID
		}

		if err := qtx.UpdateTransaction(ctx, db.UpdateTransactionParams{
			ID:             tx.ID,
			AccountID:      tx.Account.ID,
			Amount:         tx.Amount,
			KindID:         tx.Kind.ID,
			CategoryID:     catId,
			CounterpartyID: cpId,
			ToAccountID:    toAccId,
			ToAmount:       sql.NullFloat64{Float64: tx.ToAmount, Valid: tx.ToAmount != 0},
			Description:    sql.NullString{String: tx.Description, Valid: tx.Description != ""},
			ScheduledTo:    tx.ScheduledTo,
		}); err != nil {
			l.Error("updating transaction", zap.Error(err), zap.Any("new_transaction", tx), zap.Any("old_transaction", oldTx))
			return err
		}
		l.Debug("updated transaction", zap.Any("new_transaction", tx), zap.Any("old_transaction", oldTx))

		if !oldTx.FulfilledAt.IsZero() {
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
		if err := qtx.FulfillTransaction(ctx, tx.ID); err != nil {
			l.Error("fulfilling transaction", zap.Error(err), zap.String("id", tx.ID), zap.Time("fulfilled_on", tx.FulfilledAt))
			return err
		}
		l.Debug("fulfilled transaction", zap.String("id", tx.ID), zap.Time("fulfilled_on", tx.FulfilledAt))
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

	var cat *models.Category
	var cp *models.Counterparty
	var toAcc *models.Account

	if _dbTx.CategoryID.Valid {
		cat = &models.Category{
			ID:          _dbTx.CategoryID.String,
			Name:        _dbTx.CategoryName.String,
			BudgetInEgp: _dbTx.CategoryBudgetInEgp.Float64,
		}
	}

	if _dbTx.CounterpartyID.Valid {
		cp = &models.Counterparty{
			ID:   _dbTx.CounterpartyID.String,
			Name: _dbTx.CounterpartyName.String,
		}
	}

	if _dbTx.ToAccountID.Valid {
		toAcc = &models.Account{
			ID:   _dbTx.ToAccountID.String,
			Name: _dbTx.ToAccountName.String,
			Currency: models.Currency{
				ID:        _dbTx.ToCurrencyID.String,
				Enum:      models.CurrencyEnum(_dbTx.ToCurrencyEnum.Int64),
				EgpFxRate: _dbTx.ToEgpFxRate.Float64,
			},
		}
	}

	tx := models.Transaction{
		ID: _dbTx.ID,
		Account: models.Account{
			ID:   _dbTx.AccountID,
			Name: _dbTx.AccountName.String,
			Currency: models.Currency{
				ID:        _dbTx.CurrencyID.String,
				Enum:      models.CurrencyEnum(_dbTx.CurrencyEnum.Int64),
				EgpFxRate: _dbTx.EgpFxRate.Float64,
			},
		},
		Amount:         _dbTx.Amount,
		Description:    _dbTx.Description.String,
		ToAccount:      toAcc,
		ToAmount:       _dbTx.ToAmount.Float64,
		GoalTitle:      _dbTx.GoalTitle.String,
		GoalPercentage: _dbTx.GoalPercentage.(float64),
		FulfilledAt:    _dbTx.FulfilledAt.Time,
		ScheduledTo:    _dbTx.ScheduledTo,
		CreatedAt:      _dbTx.CreatedAt,
		Kind: models.TransactionKind{
			ID:              _dbTx.KindID,
			Enum:            models.TxKind(_dbTx.KindEnum.Int64),
			IncreaseBalance: _dbTx.IncreaseBalance.Bool,
		},
		Category:     cat,
		Counterparty: cp,
	}

	return &tx
}
