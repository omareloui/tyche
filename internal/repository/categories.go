package repository

import (
	"context"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
)

func (r *Repository) GetCategories(ctx context.Context) ([]models.Category, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbCats, err := q.GetCategories(ctx)
	if err != nil {
		return nil, err
	}
	return dbCategoriesRowToCategories(dbCats), nil
}

func (r *Repository) GetCategory(ctx context.Context, id string) (*models.Category, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbCat, err := q.GetCategory(ctx, id)
	if err != nil {
		return nil, err
	}
	return dbCategoryRowToCategory(&dbCat), nil
}

func (r *Repository) GetTotalBudget(ctx context.Context) (float64, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return 0, err
	}

	budget, err := q.TotalBudget(ctx)
	if err != nil {
		return 0, err
	}
	return budget, nil
}

func (r *Repository) UpdateCategory(ctx context.Context, cat *models.Category) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	return q.UpdateCategory(ctx, queries.UpdateCategoryParams{
		ID:          cat.ID,
		Name:        cat.Name,
		KindID:      cat.Kind.ID,
		BudgetInEgp: cat.BudgetInEgp,
	})
}

func (r *Repository) CreateCategory(ctx context.Context, cat *models.Category) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	return q.CreateCategory(ctx, queries.CreateCategoryParams{
		ID:          cat.ID,
		Name:        cat.Name,
		KindID:      cat.Kind.ID,
		BudgetInEgp: cat.BudgetInEgp,
	})
}

func (r *Repository) DeleteCategory(ctx context.Context, id string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	return q.DeleteCategory(ctx, id)
}

func dbCategoryToCategory(dbCat *queries.Category, kind *queries.TransactionKind) *models.Category {
	cat := &models.Category{
		ID:   dbCat.ID,
		Name: dbCat.Name,
		Kind: models.TransactionKind{
			ID: dbCat.KindID,
		},
		BudgetInEgp: dbCat.BudgetInEgp,
		CreatedAt:   dbCat.CreatedAt,
	}

	if kind != nil {
		cat.Kind = *dbTransactionKindToTransactionKind(kind)
	}

	return cat
}

func dbCategoryRowToCategory[T queries.GetCategoryRow | queries.GetCategoriesRow](dbCat *T) *models.Category {
	_dbCat := queries.GetCategoryRow(*dbCat)
	return dbCategoryToCategory(&_dbCat.Category, &_dbCat.TransactionKind)
}

func dbCategoriesRowToCategories(dbCats []queries.GetCategoriesRow) []models.Category {
	categories := make([]models.Category, len(dbCats))
	for i, cat := range dbCats {
		categories[i] = *dbCategoryRowToCategory(&cat)
	}
	return categories
}
