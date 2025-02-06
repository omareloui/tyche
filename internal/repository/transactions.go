package repository

import (
	"context"
	"database/sql"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

func (r *Repository) CreateTransaction(ctx context.Context, tx *models.Transaction, ftx *models.FrequentTx, scheduledTxs []models.Transaction) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
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

func (r *Repository) createTransactionWithTxn(ctx context.Context, qtx *queries.Queries, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)

	var catId, cpId, toAccId, goalId sql.NullString

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
	if tx.Goal != nil {
		goalId.Valid = tx.Goal.ID != ""
		goalId.String = tx.Goal.ID
	}

	if err := qtx.CreateTransaction(ctx, queries.CreateTransactionParams{
		ID:             tx.ID,
		AccountID:      tx.Account.ID,
		Amount:         tx.Amount,
		KindID:         tx.Kind.ID,
		CategoryID:     catId,
		CounterpartyID: cpId,
		Description:    sql.NullString{String: tx.Description, Valid: tx.Description != ""},
		ToAccountID:    toAccId,
		ToAmount:       sql.NullFloat64{Float64: tx.ToAmount, Valid: tx.ToAmount != 0},
		GoalID:         goalId,
		ScheduledTo:    tx.ScheduledTo,
		FulfilledAt:    sql.NullTime{Time: tx.FulfilledAt, Valid: !tx.FulfilledAt.IsZero()},
	}); err != nil {
		l.Error("create transaction", zap.Error(err), zap.Any("transaction", tx))
		return err
	}
	l.Debug("created transaction", zap.Any("transaction", tx))
	return nil
}

func (r *Repository) createTransactionAndApplyToAccountIfNeededWithTxn(ctx context.Context, qtx *queries.Queries, tx *models.Transaction) error {
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

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbTransactions, err := q.GetTransactions(ctx)
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

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbTx, err := q.GetTransaction(ctx, id)
	if err != nil {
		l.Error("querying transaction", zap.Error(err))
		return nil, err
	}
	l.Info("queried transaction")
	return dbTransactionRowToTransaction(&dbTx), nil
}

func (r *Repository) DeleteTransaction(ctx context.Context, tx *models.Transaction) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
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
	return r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)

		var catId, cpId, toAccId, goalId sql.NullString

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
		if tx.Goal != nil {
			goalId.Valid = tx.Goal.ID != ""
			goalId.String = tx.Goal.ID
		}

		if err := qtx.UpdateTransaction(ctx, queries.UpdateTransactionParams{
			ID:             tx.ID,
			AccountID:      tx.Account.ID,
			Amount:         tx.Amount,
			KindID:         tx.Kind.ID,
			CategoryID:     catId,
			CounterpartyID: cpId,
			ToAccountID:    toAccId,
			ToAmount:       sql.NullFloat64{Float64: tx.ToAmount, Valid: tx.ToAmount != 0},
			GoalID:         goalId,
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
	return r.transact(ctx, func(qtx *queries.Queries) error {
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

func dbTransactionToTransaction(
	dbTx *queries.Transaction, knd *queries.TransactionKind, acc *queries.Account,
	cur *queries.Currency, toAcc *queries.Account, toCur *queries.Currency,
	cat *queries.Category, cp *queries.Counterparty, gl *queries.Goal,
	glCur *queries.Currency,
) *models.Transaction {
	tx := models.Transaction{
		ID:          dbTx.ID,
		Account:     *dbAccountToAccount(acc, 0, cur, nil, nil),
		Amount:      dbTx.Amount,
		Description: dbTx.Description.String,
		ToAmount:    dbTx.ToAmount.Float64,
		FulfilledAt: dbTx.FulfilledAt.Time,
		ScheduledTo: dbTx.ScheduledTo,
		CreatedAt:   dbTx.CreatedAt,
		Kind:        *dbTransactionKindToTransactionKind(knd),
	}

	if toAcc != nil {
		tx.ToAccount = dbAccountToAccount(toAcc, 0, toCur, nil, nil)
	} else if dbTx.ToAccountID.Valid {
		tx.ToAccount = &models.Account{ID: dbTx.ToAccountID.String}
	}

	if cat != nil {
		tx.Category = dbCategoryToCategory(cat, knd)
	} else if dbTx.CategoryID.Valid {
		tx.Category = &models.Category{ID: dbTx.CategoryID.String}
	}

	if cp != nil {
		tx.Counterparty = dbCounterpartyToCounterparty(cp)
	} else if dbTx.CounterpartyID.Valid {
		tx.Counterparty = &models.Counterparty{ID: dbTx.CounterpartyID.String}
	}

	if gl != nil {
		tx.Goal = dbGoalToGoal(gl, glCur)
		tx.GoalPercentage = tx.Percentage(tx.Goal.Price, tx.Goal.Currency.Enum)
	} else if dbTx.GoalID.Valid {
		tx.Goal = &models.Goal{ID: dbTx.GoalID.String}
	}

	return &tx
}

func dbTransactionRowToTransaction[T queries.GetTransactionRow | queries.GetTransactionsRow](dbTx *T) *models.Transaction {
	_dbTx := queries.GetTransactionsRow(*dbTx)
	return dbTransactionToTransaction(
		&_dbTx.Transaction, &_dbTx.TransactionKind, &_dbTx.Account,
		&_dbTx.Currency, &_dbTx.Account_2, &_dbTx.Currency_2,
		&_dbTx.Category, &_dbTx.Counterparty, &_dbTx.Goal,
		&_dbTx.Currency_3)
}
