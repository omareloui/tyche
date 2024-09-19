package repository

import (
	"context"

	"github.com/omareloui/financial-planner/internal/db"
	"github.com/omareloui/financial-planner/internal/logger"
	"github.com/omareloui/financial-planner/internal/models"
	"go.uber.org/zap"
)

func (r *Repository) GetBalances(ctx context.Context) ([]models.Balance, error) {
	l := logger.FromCtx(ctx)
	dbBalances, err := r.q.GetBalances(ctx)
	if err != nil {
		l.Error("getting balance", zap.Error(err))
		return nil, err
	}
	return models.DBBalancesToBalances(dbBalances), nil
}

func (r *Repository) AddToBalance(ctx context.Context, currencyID string, amount float64) error {
	l := logger.FromCtx(ctx)
	if err := r.q.AddToBalance(ctx, db.AddToBalanceParams{CurrencyID: currencyID, Amount: amount}); err != nil {
		l.Error("adding to balance", zap.Error(err), zap.String("currency_id", currencyID), zap.Float64("amount", amount))
		return err
	}
	l.Debug("added to balance", zap.String("currency_id", currencyID), zap.Float64("amount", amount))
	return nil
}

func (r *Repository) RemoveFromBalance(ctx context.Context, currencyID string, amount float64) error {
	l := logger.FromCtx(ctx)
	if err := r.q.RemoveFromBalance(ctx, db.RemoveFromBalanceParams{CurrencyID: currencyID, Amount: amount}); err != nil {
		l.Error("removing from balance", zap.Error(err), zap.String("currency_id", currencyID), zap.Float64("amount", amount))
		return err
	}
	l.Debug("removed from balance", zap.String("currency_id", currencyID), zap.Float64("amount", amount))
	return nil
}

func (r *Repository) ApplyTransactionToBalance(ctx context.Context, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)
	upBalance := r.RemoveFromBalance
	if tx.Kind.IncreaseBalance {
		upBalance = r.AddToBalance
	}
	if err := upBalance(ctx, tx.Currency.ID, tx.Amount); err != nil {
		l.Error("applying transaction to balance", zap.Error(err), zap.Any("transaction", tx))
		return err
	}
	l.Debug("applied transaction to balance", zap.Any("transaction", tx))
	return nil
}

func (r *Repository) UndoTransactionToBalance(ctx context.Context, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)
	upBalance := r.AddToBalance
	if tx.Kind.IncreaseBalance {
		upBalance = r.RemoveFromBalance
	}
	if err := upBalance(ctx, tx.Currency.ID, tx.Amount); err != nil {
		l.Error("undoing transaction to balance", zap.Error(err), zap.Any("transaction", tx))
		return err
	}
	l.Debug("undone transaction to balance", zap.Any("transaction", tx))
	return nil
}

func (r *Repository) AddToBalanceWithTxn(qtx *db.Queries, ctx context.Context, currencyID string, amount float64) error {
	l := logger.FromCtx(ctx)
	if err := qtx.AddToBalance(ctx, db.AddToBalanceParams{CurrencyID: currencyID, Amount: amount}); err != nil {
		l.Error("adding to balance", zap.Error(err), zap.String("currency_id", currencyID), zap.Float64("amount", amount))
		return err
	}
	l.Debug("added to balance", zap.String("currency_id", currencyID), zap.Float64("amount", amount))
	return nil
}

func (r *Repository) RemoveFromBalanceWithTxn(qtx *db.Queries, ctx context.Context, currencyID string, amount float64) error {
	l := logger.FromCtx(ctx)
	if err := qtx.RemoveFromBalance(ctx, db.RemoveFromBalanceParams{CurrencyID: currencyID, Amount: amount}); err != nil {
		l.Error("removing from balance", zap.Error(err), zap.String("currency_id", currencyID), zap.Float64("amount", amount))
		return err
	}
	l.Debug("removed from balance", zap.String("currency_id", currencyID), zap.Float64("amount", amount))
	return nil
}

func (r *Repository) ApplyTransactionToBalanceWithTxn(tqx *db.Queries, ctx context.Context, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)
	upBalance := r.RemoveFromBalanceWithTxn
	if tx.Kind.IncreaseBalance {
		upBalance = r.AddToBalanceWithTxn
	}
	if err := upBalance(tqx, ctx, tx.Currency.ID, tx.Amount); err != nil {
		l.Error("applying transaction to balance", zap.Error(err), zap.Any("transaction", tx))
		return err
	}
	l.Debug("applied transaction to balance", zap.Any("transaction", tx))
	return nil
}

func (r *Repository) UndoTransactionToBalanceWithTxn(qtx *db.Queries, ctx context.Context, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)
	upBalance := r.AddToBalanceWithTxn
	if tx.Kind.IncreaseBalance {
		upBalance = r.RemoveFromBalanceWithTxn
	}
	if err := upBalance(qtx, ctx, tx.Currency.ID, tx.Amount); err != nil {
		l.Error("undoing transaction to balance", zap.Error(err), zap.Any("transaction", tx))
		return err
	}
	l.Debug("undone transaction to balance", zap.Any("transaction", tx))
	return nil
}
