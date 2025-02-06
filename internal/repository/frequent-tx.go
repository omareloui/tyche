package repository

import (
	"database/sql"
	"slices"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

func (r *Repository) createFrequentTxWithTxn(qtx *queries.Queries, ctx context.Context, ftx *models.FrequentTx) error {
	l := logger.FromCtx(ctx)

	var catId, cpId, toAccId, goalId sql.NullString

	if ftx.Category != nil {
		catId.Valid = ftx.Category.ID != ""
		catId.String = ftx.Category.ID
	}
	if ftx.Counterparty != nil {
		cpId.Valid = ftx.Counterparty.ID != ""
		cpId.String = ftx.Counterparty.ID
	}
	if ftx.ToAccount != nil {
		toAccId.Valid = ftx.ToAccount.ID != ""
		toAccId.String = ftx.ToAccount.ID
	}
	if ftx.Goal != nil {
		goalId.Valid = ftx.Goal.ID != ""
		goalId.String = ftx.Goal.ID
	}

	if err := qtx.CreateFrequentTx(ctx, queries.CreateFrequentTxParams{
		ID:                         ftx.ID,
		AccountID:                  ftx.Account.ID,
		FrequencyBy:                int64(ftx.FrequencyBy),
		Frequency:                  int64(ftx.Frequency),
		Amount:                     ftx.Amount,
		KindID:                     ftx.Kind.ID,
		CategoryID:                 catId,
		CounterpartyID:             cpId,
		Description:                sql.NullString{String: ftx.Description, Valid: ftx.Description != ""},
		ToAccountID:                toAccId,
		ToAmount:                   ftx.ToAmount,
		GoalID:                     goalId,
		StartsAt:                   ftx.StartsAt,
		StopsAt:                    sql.NullTime{Time: ftx.StopsAt, Valid: !ftx.StopsAt.IsZero()},
		LastTransactionScheduledTo: ftx.LastTransactionScheduledTo,
	}); err != nil {
		l.Error("creating frequent transaction", zap.Error(err), zap.Any("frequent tx", ftx))
		return err
	}
	l.Debug("created frequent transaction", zap.Any("frequent tx", ftx))
	return nil
}

func (r *Repository) createFrequentTxTransactionWithTxn(ctx context.Context, qtx *queries.Queries, ftxId, txId string) error {
	l := logger.FromCtx(ctx)
	if err := qtx.CreateFrequentTxTransaction(ctx, queries.CreateFrequentTxTransactionParams{
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

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		l.Error("getting db from context", zap.Error(err))
		return nil, err
	}

	dbFtx, err := q.GetFrequentTxs(ctx)
	if err != nil {
		l.Error("querying frequent transactions", zap.Error(err))
		return nil, err
	}
	l.Debug("queried frequent transactions")

	return dbFrequentTxsRowToFrequentTxs(dbFtx), nil
}

func (r *Repository) GetFrequentTx(ctx context.Context, id string) (*models.FrequentTx, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		l.Error("getting db from context", zap.Error(err))
		return nil, err
	}

	dbFtx, err := q.GetFrequentTx(ctx, id)
	if err != nil {
		l.Error("querying frequent transactions", zap.Error(err))
		return nil, err
	}
	l.Debug("queried frequent transactions")

	ftx := dbFrequentTxsRowToFrequentTxs(dbFtx)
	return &ftx[0], nil
}

func (r *Repository) UpdateFrequentTx(ctx context.Context, ftx *models.FrequentTx, scheduledTxs []models.Transaction) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)

		if err := qtx.DeleteScheduledTransactionsOfFrequentTx(ctx, ftx.ID); err != nil {
			l.Error("deleting scheduled transaction of transaction", zap.Error(err))
			return err
		}
		l.Debug("deleted scheduled transaction of transaction")

		var catId, cpId, toAccId, goalId sql.NullString

		if ftx.Category != nil {
			catId.Valid = ftx.Category.ID != ""
			catId.String = ftx.Category.ID
		}
		if ftx.Counterparty != nil {
			cpId.Valid = ftx.Counterparty.ID != ""
			cpId.String = ftx.Counterparty.ID
		}
		if ftx.ToAccount != nil {
			toAccId.Valid = ftx.ToAccount.ID != ""
			toAccId.String = ftx.ToAccount.ID
		}
		if ftx.Goal != nil {
			goalId.Valid = ftx.Goal.ID != ""
			goalId.String = ftx.Goal.ID
		}

		if err := qtx.UpdateFrequentTx(ctx, queries.UpdateFrequentTxParams{
			ID:                         ftx.ID,
			AccountID:                  ftx.Account.ID,
			Amount:                     ftx.Amount,
			KindID:                     ftx.Kind.ID,
			CategoryID:                 catId,
			CounterpartyID:             cpId,
			FrequencyBy:                int64(ftx.FrequencyBy),
			Frequency:                  int64(ftx.Frequency),
			Description:                sql.NullString{String: ftx.Description, Valid: ftx.Description != ""},
			ToAccountID:                toAccId,
			ToAmount:                   ftx.ToAmount,
			GoalID:                     goalId,
			StopsAt:                    sql.NullTime{Time: ftx.StopsAt, Valid: !ftx.StopsAt.IsZero()},
			LastTransactionScheduledTo: ftx.LastTransactionScheduledTo,
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
	return r.transact(ctx, func(qtx *queries.Queries) error {
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
	return r.transact(ctx, func(qtx *queries.Queries) error {
		if err := qtx.UpdateFrequentTxLastTransactionCreatedAt(ctx, queries.UpdateFrequentTxLastTransactionCreatedAtParams{
			ID:                         ftx.ID,
			LastTransactionScheduledTo: ftx.LastTransactionScheduledTo,
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

func dbFrequentTxsRowToFrequentTxs[T queries.GetFrequentTxRow | queries.GetFrequentTxsRow](dbFtxs []T) []models.FrequentTx {
	ftxs := []models.FrequentTx{}

	for _, _dbFtx := range dbFtxs {
		dbFt := queries.GetFrequentTxsRow(_dbFtx)
		ftxIdx := slices.IndexFunc(ftxs, func(ftx models.FrequentTx) bool { return ftx.ID == dbFt.FrequentTx.ID })
		if ftxIdx == -1 {
			ftxs = append(ftxs, *dbFrequentTxToFrequentTx(
				&dbFt.FrequentTx, &dbFt.TransactionKind, &dbFt.Account,
				&dbFt.Currency, &dbFt.Account_2, &dbFt.Currency_2,
				&dbFt.Category, &dbFt.Counterparty, &dbFt.Goal,
				&dbFt.Currency_5, nil))
			ftxIdx = len(ftxs) - 1
		}

		ftxs[ftxIdx].Transactions = append(ftxs[ftxIdx].Transactions, *dbTransactionToTransaction(
			&dbFt.Transaction, &dbFt.TransactionKind_2, &dbFt.Account_3,
			&dbFt.Currency_3, &dbFt.Account_4, &dbFt.Currency_4,
			&dbFt.Category_2, &dbFt.Counterparty_2, nil, nil))
	}

	return ftxs
}

func dbFrequentTxToFrequentTx(
	dbFtx *queries.FrequentTx, knd *queries.TransactionKind, acc *queries.Account,
	cur *queries.Currency, toAcc *queries.Account, toCur *queries.Currency,
	cat *queries.Category, cp *queries.Counterparty, gl *queries.Goal,
	glCur *queries.Currency, txs []queries.Transaction,
) *models.FrequentTx {
	ft := models.FrequentTx{
		ID:          dbFtx.ID,
		Account:     models.Account{ID: dbFtx.AccountID},
		Amount:      dbFtx.Amount,
		Description: dbFtx.Description.String,
		CreatedAt:   dbFtx.CreatedAt,
		Kind:        models.TransactionKind{ID: dbFtx.KindID},

		ToAmount:                   dbFtx.ToAmount.Float64,
		Frequency:                  uint(dbFtx.Frequency),
		FrequencyBy:                models.FrequencyBy(dbFtx.FrequencyBy),
		StartsAt:                   dbFtx.StartsAt,
		StopsAt:                    dbFtx.StopsAt.Time,
		LastTransactionScheduledTo: dbFtx.LastTransactionScheduledTo,
	}

	if toAcc != nil {
		ft.ToAccount = dbAccountToAccount(toAcc, 0, toCur, nil, nil)
	} else if dbFtx.ToAccountID.Valid {
		ft.ToAccount = &models.Account{ID: dbFtx.ToAccountID.String}
	}

	if cat != nil {
		ft.Category = dbCategoryToCategory(cat, knd)
	} else if dbFtx.CategoryID.Valid {
		ft.Category = &models.Category{ID: dbFtx.CategoryID.String}
	}

	if cp != nil {
		ft.Counterparty = dbCounterpartyToCounterparty(cp)
	} else if dbFtx.CounterpartyID.Valid {
		ft.Counterparty = &models.Counterparty{ID: dbFtx.CounterpartyID.String}
	}

	if gl != nil {
		ft.Goal = dbGoalToGoal(gl, glCur)
	} else if dbFtx.GoalID.Valid {
		ft.Goal = &models.Goal{ID: dbFtx.GoalID.String}
	}

	for _, tx := range txs {
		ft.Transactions = append(ft.Transactions, *dbTransactionToTransaction(&tx, nil, nil, nil, nil, nil, nil, nil, nil, nil))
	}

	return &ft
}
