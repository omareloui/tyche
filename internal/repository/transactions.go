package repository

import (
	"context"
	"database/sql"

	"github.com/omareloui/financial-planner/internal/db"
	"github.com/omareloui/financial-planner/internal/models"
)

func (r *Repository) CreateTransaction(ctx context.Context, t *models.Transaction) error {
	err := r.db.CreateTransaction(ctx, db.CreateTransactionParams{
		ID:          t.ID,
		Amount:      t.Amount,
		KindID:      t.Kind.ID,
		CurrencyID:  t.Currency.ID,
		IsRecurrent: t.IsRecurrent,
		ScheduledTo: t.ScheduledTo,
		CreatedAt:   t.CreatedAt,
	})
	if err != nil {
		return err
	}
	return nil
}

func (r *Repository) GetTransactions(ctx context.Context) ([]models.Transaction, error) {
	dbTransactions, err := r.db.GetTransactions(ctx)
	if err != nil {
		return nil, err
	}

	tx := make([]models.Transaction, len(dbTransactions))
	for i, t := range dbTransactions {
		tx[i] = *models.DBTransactionsRawToTransaction(&t)
	}
	return tx, nil
}

func (r *Repository) GetTransaction(ctx context.Context, id string) (*models.Transaction, error) {
	dbTx, err := r.db.GetTransaction(ctx, id)
	if err != nil {
		return nil, err
	}
	return models.DBTransactionRawToTransaction(&dbTx), nil
}

func (r *Repository) DeleteTransaction(ctx context.Context, id string) error {
	return r.db.DeleteTransaction(ctx, id)
}

func (r *Repository) UpdateTransaction(ctx context.Context, tx *models.Transaction) error {
	return r.db.UpdateTransaction(ctx, db.UpdateTransactionParams{
		ID:          tx.ID,
		Amount:      tx.Amount,
		CurrencyID:  tx.Currency.ID,
		KindID:      tx.Kind.ID,
		IsRecurrent: tx.IsRecurrent,
		FulfilledOn: sql.NullTime{Time: tx.FulfilledOn, Valid: !tx.FulfilledOn.IsZero()},
		ScheduledTo: tx.ScheduledTo,
	})
}
