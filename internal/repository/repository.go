package repository

import (
	"context"

	"github.com/omareloui/financial-plans/internal/db"
	"github.com/omareloui/financial-plans/internal/models"
)

type repository struct {
	db *db.Queries
}

func New(db *db.Queries) *repository {
	return &repository{db: db}
}

func (r *repository) GetCurrencies(ctx context.Context) ([]models.Currency, error) {
	dbCurrencies, err := r.db.GetCurrencies(ctx)
	if err != nil {
		return nil, err
	}
	return models.DBCurrenciesToCurrencies(dbCurrencies), nil
}
