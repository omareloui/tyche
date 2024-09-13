package repository

import (
	"context"

	"github.com/omareloui/financial-plans/internal/db"
	"github.com/omareloui/financial-plans/internal/models"
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
