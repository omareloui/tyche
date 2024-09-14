package repository

import (
	"context"

	"github.com/omareloui/financial-planner/internal/models"
)

func (r *Repository) GetTransactionKinds(ctx context.Context) ([]models.TransactionKind, error) {
	dbKinds, err := r.db.GetTransactionKinds(ctx)
	if err != nil {
		return nil, err
	}
	return models.DBTransactionKindsToTransactionKinds(dbKinds), nil
}

func (r *Repository) GetTransactionKind(ctx context.Context, id string) (*models.TransactionKind, error) {
	k, err := r.db.GetTransactionKind(ctx, id)
	if err != nil {
		return nil, err
	}
	return models.DBTransactionKindToTransactionKind(&k), nil
}

func (r *Repository) GetTransactionKindsMap(ctx context.Context) (map[string]string, error) {
	kinds, err := r.GetTransactionKinds(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(kinds))
	for _, k := range kinds {
		m[k.ID] = k.Kind
	}
	return m, nil
}

func (r *Repository) TransactionKindsToMap(kinds []models.TransactionKind) map[string]string {
	m := make(map[string]string, len(kinds))
	for _, k := range kinds {
		m[k.ID] = k.Kind
	}
	return m
}
