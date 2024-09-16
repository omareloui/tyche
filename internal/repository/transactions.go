package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/omareloui/financial-planner/internal/db"
	"github.com/omareloui/financial-planner/internal/models"
)

func (r *Repository) CreateTransaction(ctx context.Context, t *models.Transaction) error {
	err := r.q.CreateTransaction(ctx, db.CreateTransactionParams{
		ID:          t.ID,
		Amount:      t.Amount,
		KindID:      t.Kind.ID,
		CurrencyID:  t.Currency.ID,
		Description: sql.NullString{String: t.Description, Valid: t.Description != ""},
		IsRecurrent: t.IsRecurrent,
		ScheduledTo: t.ScheduledTo,
		FulfilledOn: sql.NullTime{Time: t.FulfilledOn, Valid: !t.FulfilledOn.IsZero()},
		CreatedAt:   t.CreatedAt,
	})
	if err != nil {
		return err
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
	return r.q.DeleteTransaction(ctx, id)
}

func (r *Repository) UpdateTransaction(ctx context.Context, tx *models.Transaction) error {
	return r.q.UpdateTransaction(ctx, db.UpdateTransactionParams{
		ID:          tx.ID,
		Amount:      tx.Amount,
		CurrencyID:  tx.Currency.ID,
		KindID:      tx.Kind.ID,
		IsRecurrent: tx.IsRecurrent,
		ScheduledTo: tx.ScheduledTo,
	})
}

func (r *Repository) FulfillTransaction(ctx context.Context, id string, fulfilledOn time.Time) error {
	return r.q.FulfillTransaction(ctx, db.FulfillTransactionParams{
		ID:          id,
		FulfilledOn: sql.NullTime{Time: fulfilledOn, Valid: !fulfilledOn.IsZero()},
	})
}
