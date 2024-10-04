package repository

import (
	"context"

	"github.com/omareloui/tyche/internal/db"
	"github.com/omareloui/tyche/internal/models"
)

func (r *Repository) GetCategories(ctx context.Context) ([]models.Category, error) {
	dbCats, err := r.q.GetCategories(ctx)
	if err != nil {
		return nil, err
	}
	return dbCategoriesRowToCategories(dbCats), nil
}

func (r *Repository) GetCategory(ctx context.Context, id string) (*models.Category, error) {
	dbCat, err := r.q.GetCategory(ctx, id)
	if err != nil {
		return nil, err
	}
	return dbCategoryRowToCategory(&dbCat), nil
}

func (r *Repository) GetTotalBudget(ctx context.Context) (float64, error) {
	budget, err := r.q.TotalBudget(ctx)
	if err != nil {
		return 0, err
	}
	return budget, nil
}

func (r *Repository) UpdateCategory(ctx context.Context, cat *models.Category) error {
	return r.q.UpdateCategory(ctx, db.UpdateCategoryParams{
		ID:          cat.ID,
		Name:        cat.Name,
		KindID:      cat.Kind.ID,
		BudgetInEgp: cat.BudgetInEgp,
	})
}

func (r *Repository) CreateCategory(ctx context.Context, cat *models.Category) error {
	return r.q.CreateCategory(ctx, db.CreateCategoryParams{
		ID:          cat.ID,
		Name:        cat.Name,
		KindID:      cat.Kind.ID,
		BudgetInEgp: cat.BudgetInEgp,
	})
}

func (r *Repository) DeleteCategory(ctx context.Context, id string) error {
	return r.q.DeleteCategory(ctx, id)
}

func dbCategoryRowToCategory[T db.GetCategoryRow | db.GetCategoriesRow](dbCat *T) *models.Category {
	_dbCat := db.GetCategoryRow(*dbCat)
	return &models.Category{
		ID:   _dbCat.ID,
		Name: _dbCat.Name,
		Kind: models.TransactionKind{
			ID:              _dbCat.KindID,
			Kind:            _dbCat.Kind.String,
			IncreaseBalance: _dbCat.IncreaseBalance.Bool,
		},
		BudgetInEgp: _dbCat.BudgetInEgp,
		CreatedAt:   _dbCat.CreatedAt,
	}
}

func dbCategoriesRowToCategories(dbCats []db.GetCategoriesRow) []models.Category {
	categories := make([]models.Category, len(dbCats))
	for i, cat := range dbCats {
		categories[i] = *dbCategoryRowToCategory(&cat)
	}
	return categories
}
