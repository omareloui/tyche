package repository

import (
	"context"

	"github.com/omareloui/financial-planner/internal/db"
	"github.com/omareloui/financial-planner/internal/models"
	"go.uber.org/zap"
)

func (r *Repository) GetBalances(ctx context.Context) ([]models.Balance, error) {
	dbBalances, err := r.q.GetBalances(ctx)
	if err != nil {
		r.l.Error("getting balance", zap.Error(err))
		return nil, err
	}
	return models.DBBalancesToBalances(dbBalances), nil
}

func (r *Repository) AddToBalance(ctx context.Context, currencyID string, amount float64) error {
	if err := r.q.AddToBalance(ctx, db.AddToBalanceParams{CurrencyID: currencyID, Amount: amount}); err != nil {
		r.l.Error("adding to balance", zap.Error(err), zap.String("currency_id", currencyID), zap.Float64("amount", amount))
		return err
	}
	r.l.Debug("added to balance", zap.String("currency_id", currencyID), zap.Float64("amount", amount))
	return nil
}

func (r *Repository) RemoveFromBalance(ctx context.Context, currencyID string, amount float64) error {
	if err := r.q.RemoveFromBalance(ctx, db.RemoveFromBalanceParams{CurrencyID: currencyID, Amount: amount}); err != nil {
		r.l.Error("removing from balance", zap.Error(err), zap.String("currency_id", currencyID), zap.Float64("amount", amount))
		return err
	}
	r.l.Debug("removed from balance", zap.String("currency_id", currencyID), zap.Float64("amount", amount))
	return nil
}

func (r *Repository) ApplyTransactionToBalance(ctx context.Context, tx *models.Transaction) error {
	upBalance := r.RemoveFromBalance
	if tx.Kind.IncreaseBalance {
		upBalance = r.AddToBalance
	}
	if err := upBalance(ctx, tx.Currency.ID, tx.Amount); err != nil {
		r.l.Error("applying transaction to balance", zap.Error(err), zap.Any("transaction", tx))
		return err
	}
	r.l.Debug("applied transaction to balance", zap.Any("transaction", tx))
	return nil
}

func (r *Repository) UndoTransactionToBalance(ctx context.Context, tx *models.Transaction) error {
	upBalance := r.AddToBalance
	if tx.Kind.IncreaseBalance {
		upBalance = r.RemoveFromBalance
	}
	if err := upBalance(ctx, tx.Currency.ID, tx.Amount); err != nil {
		r.l.Error("undoing transaction to balance", zap.Error(err), zap.Any("transaction", tx))
		return err
	}
	r.l.Debug("undone transaction to balance", zap.Any("transaction", tx))
	return nil
}

func (r *Repository) AddToBalanceWithTXN(txn *db.Queries, ctx context.Context, currencyID string, amount float64) error {
	if err := txn.AddToBalance(ctx, db.AddToBalanceParams{CurrencyID: currencyID, Amount: amount}); err != nil {
		r.l.Error("adding to balance", zap.Error(err), zap.String("currency_id", currencyID), zap.Float64("amount", amount))
		return err
	}
	r.l.Debug("added to balance", zap.String("currency_id", currencyID), zap.Float64("amount", amount))
	return nil
}

func (r *Repository) RemoveFromBalanceWithTXN(txn *db.Queries, ctx context.Context, currencyID string, amount float64) error {
	if err := txn.RemoveFromBalance(ctx, db.RemoveFromBalanceParams{CurrencyID: currencyID, Amount: amount}); err != nil {
		r.l.Error("removing from balance", zap.Error(err), zap.String("currency_id", currencyID), zap.Float64("amount", amount))
		return err
	}
	r.l.Debug("removed from balance", zap.String("currency_id", currencyID), zap.Float64("amount", amount))
	return nil
}

func (r *Repository) ApplyTransactionToBalanceWithTXN(txn *db.Queries, ctx context.Context, tx *models.Transaction) error {
	upBalance := r.RemoveFromBalanceWithTXN
	if tx.Kind.IncreaseBalance {
		upBalance = r.AddToBalanceWithTXN
	}
	if err := upBalance(txn, ctx, tx.Currency.ID, tx.Amount); err != nil {
		r.l.Error("applying transaction to balance", zap.Error(err), zap.Any("transaction", tx))
		return err
	}
	r.l.Debug("applied transaction to balance", zap.Any("transaction", tx))
	return nil
}

func (r *Repository) UndoTransactionToBalanceWithTXN(txn *db.Queries, ctx context.Context, tx *models.Transaction) error {
	upBalance := r.AddToBalanceWithTXN
	if tx.Kind.IncreaseBalance {
		upBalance = r.RemoveFromBalanceWithTXN
	}
	if err := upBalance(txn, ctx, tx.Currency.ID, tx.Amount); err != nil {
		r.l.Error("undoing transaction to balance", zap.Error(err), zap.Any("transaction", tx))
		return err
	}
	r.l.Debug("undone transaction to balance", zap.Any("transaction", tx))
	return nil
}
