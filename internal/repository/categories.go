package repository

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
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
	return dbCategoriesRowToCategories(dbCats)
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

	kinds, err := marshalTxKinds(cat.Kinds)
	if err != nil {
		return err
	}

	return q.UpdateCategory(ctx, queries.UpdateCategoryParams{
		ID:               cat.ID,
		Name:             cat.Name,
		Color:            cat.Color,
		Kinds:            kinds,
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

	kinds, err := marshalTxKinds(cat.Kinds)
	if err != nil {
		return err
	}

	return q.CreateCategory(ctx, queries.CreateCategoryParams{
		ID:               cat.ID,
		Name:             cat.Name,
		Color:            cat.Color,
		Kinds:            kinds,
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

func dbCategoryToCategory(dbCat *queries.Category, cur *queries.Currency) *models.Category {
	cat := &models.Category{
		ID:           dbCat.ID,
		Name:         dbCat.Name,
		Color:        dbCat.Color,
		BudgetAmount: dbCat.BudgetAmount.Float64,
		BudgetCurrency: &models.Currency{
			ID: dbCat.BudgetCurrencyID.String,
		},
		CreatedAt: dbCat.CreatedAt,
	}

	cat.Kinds, _ = unmarshalTxKinds(dbCat.Kinds)

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
	return dbCategoryToCategory(&_dbCat.Category, &queries.Currency{
		ID:     _dbCat.Category.BudgetCurrencyID.String,
		String: _dbCat.BudgetCurrencyString.String,
		Symbol: _dbCat.BudgetCurrencySymbol.String,
	})
}

func dbCategoriesRowToCategories(dbCats []queries.GetCategoriesRow) ([]models.Category, error) {
	categories := make([]models.Category, len(dbCats))
	for i, cat := range dbCats {
		categories[i] = *dbCategoryRowToCategory(&cat)
	}
	return categories, nil
}

func marshalTxKinds(kinds []models.TxKind) ([]byte, error) {
	marshaledKinds, err := json.Marshal(kinds)
	if err != nil {
		logger.Get().Error("error on marshaling category kinds for db", zap.Error(err))
		return nil, err
	}
	return marshaledKinds, nil
}

func unmarshalTxKinds(dbKinds []byte) ([]models.TxKind, error) {
	if dbKinds == nil {
		return nil, nil
	}
	kinds := []models.TxKind{}
	err := json.Unmarshal(dbKinds, &kinds)
	if err != nil {
		logger.Get().Error("error on unmarshaling category kinds from db", zap.Error(err))
		return nil, err
	}
	return kinds, nil
}
