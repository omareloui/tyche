package repository

import (
	"context"
	"time"

	"github.com/omareloui/financial-planner/internal/db"
	"github.com/omareloui/financial-planner/internal/models"
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

func (r *Repository) UpdateCategory(ctx context.Context, cat *models.Category) error {
	return r.q.UpdateCategory(ctx, db.UpdateCategoryParams{
		ID:          cat.ID,
		Name:        cat.Name,
		BudgetInEgp: cat.BudgetInEgp,
	})
}

func (r *Repository) CreateCategory(ctx context.Context, cat *models.Category) error {
	return r.q.CreateCategory(ctx, db.CreateCategoryParams{
		ID:          cat.ID,
		Name:        cat.Name,
		BudgetInEgp: cat.BudgetInEgp,
		CreatedAt:   cat.CreatedAt,
	})
}

func (r *Repository) DeleteCategory(ctx context.Context, id string) error {
	return r.q.DeleteCategory(ctx, id)
}

func dbCategoryRowToCategory(dbCat *db.Category) *models.Category {
	return &models.Category{
		ID:          dbCat.ID,
		Name:        dbCat.Name,
		BudgetInEgp: dbCat.BudgetInEgp,
		CreatedAt:   dbCat.CreatedAt.(time.Time),
	}
}

func dbCategoriesRowToCategories(dbCats []db.Category) []models.Category {
	categories := make([]models.Category, len(dbCats))
	for i, cat := range dbCats {
		categories[i] = *dbCategoryRowToCategory(&cat)
	}
	return categories
}
