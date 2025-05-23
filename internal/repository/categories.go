package repository

import (
	"context"
	"database/sql"

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

func (r *Repository) UpdateCategory(ctx context.Context, cat *models.Category) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	var curId string
	if cat.BudgetCurrency != nil {
		curId = cat.BudgetCurrency.ID
	}

	return q.UpdateCategory(ctx, queries.UpdateCategoryParams{
		ID:               cat.ID,
		Name:             cat.Name,
		KindID:           cat.Kind.ID,
		BudgetAmount:     sql.NullFloat64{Float64: cat.BudgetAmount, Valid: cat.BudgetAmount != 0},
		BudgetCurrencyID: sql.NullString{String: curId, Valid: curId != ""},
	})
}

func (r *Repository) CreateCategory(ctx context.Context, cat *models.Category) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	var curId string
	if cat.BudgetCurrency != nil {
		curId = cat.BudgetCurrency.ID
	}

	return q.CreateCategory(ctx, queries.CreateCategoryParams{
		ID:               cat.ID,
		Name:             cat.Name,
		KindID:           cat.Kind.ID,
		BudgetAmount:     sql.NullFloat64{Float64: cat.BudgetAmount, Valid: cat.BudgetAmount != 0},
		BudgetCurrencyID: sql.NullString{String: curId, Valid: curId != ""},
	})
}

func (r *Repository) DeleteCategory(ctx context.Context, id string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	return q.DeleteCategory(ctx, id)
}

func dbCategoryToCategory(dbCat *queries.Category, kind *queries.TransactionKind, cur *queries.Currency) *models.Category {
	cat := &models.Category{
		ID:   dbCat.ID,
		Name: dbCat.Name,
		Kind: models.TransactionKind{
			ID: dbCat.KindID,
		},
		BudgetAmount: dbCat.BudgetAmount.Float64,
		BudgetCurrency: &models.Currency{
			ID: dbCat.BudgetCurrencyID.String,
		},
		CreatedAt: dbCat.CreatedAt,
	}

	if kind != nil {
		cat.Kind = *dbTransactionKindToTransactionKind(kind)
	}

	if cur != nil {
		cat.BudgetCurrency.String = cur.String
		cat.BudgetCurrency.Symbol = cur.Symbol
		cat.BudgetCurrency.IbanPrefix = cur.IbanPrefix.String
		cat.BudgetCurrency.Kind = models.CurrencyKind(cur.Kind)
	}

	return cat
}

func dbCategoryRowToCategory[T queries.GetCategoryRow | queries.GetCategoriesRow](dbCat *T) *models.Category {
	_dbCat := queries.GetCategoryRow(*dbCat)
	return dbCategoryToCategory(&_dbCat.Category, &_dbCat.TransactionKind, &queries.Currency{
		ID:     _dbCat.Category.BudgetCurrencyID.String,
		String: _dbCat.BudgetCurrencyString.String,
		Symbol: _dbCat.BudgetCurrencySymbol.String,
	})
}

func dbCategoriesRowToCategories(dbCats []queries.GetCategoriesRow) []models.Category {
	categories := make([]models.Category, len(dbCats))
	for i, cat := range dbCats {
		categories[i] = *dbCategoryRowToCategory(&cat)
	}
	return categories
}
