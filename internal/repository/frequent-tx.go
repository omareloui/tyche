package repository

import (
	"database/sql"
	"slices"

	"github.com/omareloui/financial-planner/internal/db"
	"github.com/omareloui/financial-planner/internal/logger"
	"github.com/omareloui/financial-planner/internal/models"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

func (r *Repository) createFrequentTxWithTxn(qtx *db.Queries, ctx context.Context, ftx *models.FrequentTx) error {
	l := logger.FromCtx(ctx)

	var catId sql.NullString
	if ftx.Category != nil {
		catId = sql.NullString{Valid: ftx.Category.ID != "", String: ftx.Category.ID}
	} else {
		catId = sql.NullString{Valid: false}
	}

	if err := qtx.CreateFrequentTx(ctx, db.CreateFrequentTxParams{
		ID:                       ftx.ID,
		AccountID:                ftx.Account.ID,
		FrequencyBy:              int64(ftx.FrequencyBy),
		Frequency:                int64(ftx.Frequency),
		Amount:                   ftx.Amount,
		KindID:                   ftx.Kind.ID,
		CategoryID:               catId,
		Description:              sql.NullString{String: ftx.Description, Valid: ftx.Description != ""},
		StartsOn:                 ftx.StartsOn,
		StopsOn:                  sql.NullTime{Time: ftx.StopsOn, Valid: !ftx.StopsOn.IsZero()},
		LastTransactionCreatedAt: ftx.LastTransactionCreatedAt,
	}); err != nil {
		l.Error("creating frequent transaction", zap.Error(err), zap.Any("frequent tx", ftx))
		return err
	}
	l.Debug("created frequent transaction", zap.Any("frequent tx", ftx))
	return nil
}

func (r *Repository) createFrequentTxTransactionWithTxn(ctx context.Context, qtx *db.Queries, ftxId, txId string) error {
	l := logger.FromCtx(ctx)
	if err := qtx.CreateFrequentTxTransaction(ctx, db.CreateFrequentTxTransactionParams{
		TransactionID: txId,
		FrequencyID:   ftxId,
	}); err != nil {
		l.Error("creating frequent transaction transaction", zap.Error(err))
		return err
	}
	l.Debug("created frequent transaction transaction")
	return nil
}

func (r *Repository) GetFrequentTxs(ctx context.Context) ([]models.FrequentTx, error) {
	l := logger.FromCtx(ctx)
	dbFtx, err := r.q.GetFrequentTxs(ctx)
	if err != nil {
		l.Error("querying frequent transactions", zap.Error(err))
		return nil, err
	}
	l.Debug("queried frequent transactions")

	return dbFrequentTxsRowToFrequentTxs(dbFtx), nil
}

func (r *Repository) GetFrequentTx(ctx context.Context, id string) (*models.FrequentTx, error) {
	l := logger.FromCtx(ctx)
	dbFtx, err := r.q.GetFrequentTx(ctx, id)
	if err != nil {
		l.Error("querying frequent transactions", zap.Error(err))
		return nil, err
	}
	l.Debug("queried frequent transactions")

	ftx := dbFrequentTxsRowToFrequentTxs(dbFrequentTxRowToDBFrequentTxsRow(dbFtx))
	return &ftx[0], nil
}

func (r *Repository) UpdateFrequentTx(ctx context.Context, ftx *models.FrequentTx, scheduledTxs []models.Transaction) error {
	return r.transact(func(qtx *db.Queries) error {
		l := logger.FromCtx(ctx)

		if err := qtx.DeleteScheduledTransactionsOfFrequentTx(ctx, ftx.ID); err != nil {
			l.Error("deleting scheduled transaction of transaction", zap.Error(err))
			return err
		}
		l.Debug("deleted scheduled transaction of transaction")

		var catId sql.NullString
		if ftx.Category != nil {
			catId = sql.NullString{Valid: ftx.Category.ID != "", String: ftx.Category.ID}
		} else {
			catId = sql.NullString{Valid: false}
		}

		if err := qtx.UpdateFrequentTx(ctx, db.UpdateFrequentTxParams{
			ID:                       ftx.ID,
			AccountID:                ftx.Account.ID,
			Amount:                   ftx.Amount,
			KindID:                   ftx.Kind.ID,
			CategoryID:               catId,
			FrequencyBy:              int64(ftx.FrequencyBy),
			Frequency:                int64(ftx.Frequency),
			Description:              sql.NullString{String: ftx.Description, Valid: ftx.Description != ""},
			StopsOn:                  sql.NullTime{Time: ftx.StopsOn, Valid: !ftx.StopsOn.IsZero()},
			LastTransactionCreatedAt: ftx.LastTransactionCreatedAt,
		}); err != nil {
			l.Error("updating frequent tx", zap.Any("frequent tx", ftx), zap.Error(err))
			return err
		}
		l.Debug("updated frequent tx", zap.Any("frequent tx", ftx))

		for _, stx := range scheduledTxs {
			if err := r.createTransactionWithTxn(ctx, qtx, &stx); err != nil {
				return err
			}
			if err := r.createFrequentTxTransactionWithTxn(ctx, qtx, ftx.ID, stx.ID); err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *Repository) DeleteFrequentTx(ctx context.Context, id string) error {
	return r.transact(func(qtx *db.Queries) error {
		l := logger.FromCtx(ctx)

		if err := qtx.DeleteScheduledTransactionsOfFrequentTx(ctx, id); err != nil {
			l.Error("deleting scheduled transaction of transaction", zap.Error(err))
			return err
		}
		l.Debug("deleted scheduled transaction of transaction")

		if err := qtx.DeleteFrequentTx(ctx, id); err != nil {
			l.Error("deleting frequent transaction", zap.Error(err))
			return err
		}
		l.Debug("deleted frequent transaction")

		return nil
	})
}

func (r *Repository) GenerateScheduledTransactions(ctx context.Context, ftx *models.FrequentTx, scheduledTxs []models.Transaction) error {
	return r.transact(func(qtx *db.Queries) error {
		if err := qtx.UpdateFrequentTxLastTransactionCreatedAt(ctx, db.UpdateFrequentTxLastTransactionCreatedAtParams{
			ID:                       ftx.ID,
			LastTransactionCreatedAt: ftx.LastTransactionCreatedAt,
		}); err != nil {
			return err
		}
		for _, stx := range scheduledTxs {
			if err := r.createTransactionWithTxn(ctx, qtx, &stx); err != nil {
				return err
			}
			if err := r.createFrequentTxTransactionWithTxn(ctx, qtx, ftx.ID, stx.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

func dbFrequentTxsRowToFrequentTxs(dbFtxs []db.GetFrequentTxsRow) []models.FrequentTx {
	ftxs := []models.FrequentTx{}

	for _, dbFtx := range dbFtxs {
		ftxIdx := slices.IndexFunc(ftxs, func(ftx models.FrequentTx) bool { return ftx.ID == dbFtx.ID })
		if ftxIdx == -1 {
			var cat *models.Category

			if dbFtx.CategoryID.Valid {
				cat = &models.Category{
					ID:          dbFtx.CategoryID.String,
					Name:        dbFtx.CategoryName.String,
					BudgetInEgp: dbFtx.CategoryBudgetInEgp.Float64,
				}
			}

			ftxs = append(ftxs, models.FrequentTx{
				ID: dbFtx.ID,
				Account: models.Account{
					ID:   dbFtx.AccountID,
					Name: dbFtx.AccountName.String,
					Currency: models.Currency{
						ID:        dbFtx.CurrencyID.String,
						Enum:      models.CurrencyEnum(dbFtx.CurrencyEnum.Int64),
						EgpFxRate: dbFtx.EgpFxRate.Float64,
					},
				},
				Amount:      dbFtx.Amount,
				Description: dbFtx.Description.String,
				CreatedAt:   dbFtx.CreatedAt,
				Kind: models.TransactionKind{
					ID:              dbFtx.KindID,
					Kind:            dbFtx.Kind.String,
					IncreaseBalance: dbFtx.IncreaseBalance.Bool,
				},
				Category:                 cat,
				Frequency:                uint(dbFtx.Frequency),
				FrequencyBy:              models.FrequencyBy(dbFtx.FrequencyBy),
				StartsOn:                 dbFtx.StartsOn,
				StopsOn:                  dbFtx.StopsOn.Time,
				LastTransactionCreatedAt: dbFtx.LastTransactionCreatedAt,
				Transactions:             []models.Transaction{},
			})
			ftxIdx = len(ftxs) - 1
		}

		if !dbFtx.TransactionID.Valid {
			continue
		}

		var tCat *models.Category

		if !dbFtx.TransactionCurrencyID.Valid {
			tCat = &models.Category{
				ID:          dbFtx.TransactionCurrencyID.String,
				Name:        dbFtx.TransactionCategoryName.String,
				BudgetInEgp: dbFtx.TransactionCategoryBudgetInEgp.Float64,
			}
		}

		ftxs[ftxIdx].Transactions = append(ftxs[ftxIdx].Transactions, models.Transaction{
			ID: dbFtx.TransactionID.String,
			Account: models.Account{
				ID:   dbFtx.TransactionAccountID.String,
				Name: dbFtx.AccountName.String,
				Currency: models.Currency{
					ID:        dbFtx.CurrencyID.String,
					Enum:      models.CurrencyEnum(dbFtx.CurrencyEnum.Int64),
					EgpFxRate: dbFtx.EgpFxRate.Float64,
				},
			},
			Amount:      dbFtx.TransactionAmount.Float64,
			Description: dbFtx.TransactionDescription.String,
			FulfilledOn: dbFtx.TransactionFulfilledOn.Time,
			ScheduledTo: dbFtx.TransactionScheduledTo.Time,
			CreatedAt:   dbFtx.TransactionCreatedAt.Time,
			Kind: models.TransactionKind{
				ID:              dbFtx.TransactionKindID.String,
				Kind:            dbFtx.TransactionKind.String,
				IncreaseBalance: dbFtx.TransactionIncreaseBalance.Bool,
			},
			Category: tCat,
		})
	}

	return ftxs
}

func dbFrequentTxRowToDBFrequentTxsRow(ftx []db.GetFrequentTxRow) []db.GetFrequentTxsRow {
	ftxs := make([]db.GetFrequentTxsRow, len(ftx))
	for i, f := range ftx {
		ftxs[i] = db.GetFrequentTxsRow(f)
	}
	return ftxs
}
